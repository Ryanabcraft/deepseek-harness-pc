package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	webview "github.com/webview/webview_go"
)

//go:embed ui.html
var uiHTML string

const (
	repoURL    = "https://github.com/Ryanabcraft/deepseek-harness-pc"
	assetURL   = repoURL + "/releases/latest/download/DeepSeek-Harness.exe"
	appName    = "DeepSeek Harness"
	exeName    = "DeepSeek Harness.exe"
	installDir = `%LOCALAPPDATA%\Programs\DeepSeekHarness`
)

var (
	app webview.WebView
)

func emit(js string) {
	app.Dispatch(func() { app.Eval(js) })
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func step(id, state, detail string) {
	emit(fmt.Sprintf("window.step(%s,%s,%s)", jsonStr(id), jsonStr(state), jsonStr(detail)))
}

func runHidden(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func has(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func refreshPath() {
	machine := os.Getenv("PATH")
	_ = machine
	ps := `[Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')`
	out, err := runHidden("powershell", "-NoProfile", "-Command", ps)
	if err == nil && strings.TrimSpace(out) != "" {
		os.Setenv("PATH", strings.TrimSpace(out))
	}
}

func localAppData() string {
	return os.Getenv("LOCALAPPDATA")
}

func ensureNode() (string, error) {
	if has("node") {
		out, _ := runHidden("node", "--version")
		return out, nil
	}
	step("node", "run", "")
	if !has("winget") {
		return "", fmt.Errorf("winget not available")
	}
	_, err := runHidden("winget", "install", "--id", "OpenJS.NodeJS.LTS", "-e", "--silent",
		"--accept-package-agreements", "--accept-source-agreements")
	refreshPath()
	if !has("node") {
		if _, statErr := os.Stat(`C:\Program Files\nodejs\node.exe`); statErr == nil {
			os.Setenv("PATH", `C:\Program Files\nodejs;`+os.Getenv("PATH"))
		}
	}
	if err != nil && !has("node") {
		return "", err
	}
	if !has("node") {
		return "", fmt.Errorf("node not on PATH after install")
	}
	out, _ := runHidden("node", "--version")
	return out, nil
}

func npmCmd() string {
	if has("npm.cmd") {
		return "npm.cmd"
	}
	return "npm"
}

func ensureDsh() (string, error) {
	if has("dsh") {
		return "installed", nil
	}
	step("dsh", "run", "")
	if _, err := runHidden(npmCmd(), "install", "-g", "@deepseek-ai/dsh"); err != nil {
		return "", err
	}
	refreshPath()
	if !has("dsh") {
		return "", fmt.Errorf("dsh not on PATH after install")
	}
	return "installed", nil
}

func ensureOpencode() (string, error) {
	if has("opencode") {
		return "skip", nil
	}
	step("opencode", "run", "")
	if _, err := runHidden(npmCmd(), "install", "-g", "@opencode/cli"); err != nil {
		return "", err
	}
	return "installed", nil
}

func download(url, target string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := target + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	written := int64(0)
	buf := make([]byte, 64*1024)
	last := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			written += int64(n)
			if time.Since(last) > 120*time.Millisecond {
				last = time.Now()
				pct := 0
				if total > 0 {
					pct = int(written * 100 / total)
				}
				emit(fmt.Sprintf("window.progress(%d,%d,%d)", written, total, pct))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			return readErr
		}
	}
	f.Close()
	return os.Rename(tmp, target)
}

func createShortcut(target string) error {
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	lnk := filepath.Join(desktop, appName+".lnk")
	script := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut(%s);$s.TargetPath=%s;$s.WorkingDirectory=%s;$s.IconLocation=%s;$s.Save()`,
		psQuote(lnk), psQuote(target), psQuote(filepath.Dir(target)), psQuote(target+",0"))
	_, err := runHidden("powershell", "-NoProfile", "-Command", script)
	return err
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func installAll(optsJSON string) {
	var opts struct {
		Opencode bool `json:"opencode"`
	}
	_ = json.Unmarshal([]byte(optsJSON), &opts)

	// Node
	if nodeVer, err := ensureNode(); err != nil {
		step("node", "fail", err.Error())
		emit("window.fail(" + jsonStr("node") + ")")
		return
	} else {
		step("node", "ok", nodeVer)
	}

	// DSH
	if _, err := ensureDsh(); err != nil {
		step("dsh", "fail", err.Error())
		emit("window.fail(" + jsonStr("dsh") + ")")
		return
	}
	step("dsh", "ok", "")

	// OpenCode (optional)
	if opts.Opencode {
		if state, err := ensureOpencode(); err != nil {
			step("opencode", "fail", err.Error())
			emit("window.fail(" + jsonStr("opencode") + ")")
			return
		} else if state == "skip" {
			step("opencode", "skip", "")
		} else {
			step("opencode", "ok", "")
		}
	} else {
		step("opencode", "skip", "")
	}

	// App
	step("app", "run", "")
	dir := filepath.Join(localAppData(), "Programs", "DeepSeekHarness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		step("app", "fail", err.Error())
		emit("window.fail(" + jsonStr("app") + ")")
		return
	}
	target := filepath.Join(dir, exeName)
	if err := download(assetURL, target); err != nil {
		step("app", "fail", err.Error())
		emit("window.fail(" + jsonStr("app") + ")")
		return
	}
	step("app", "ok", "")

	if err := createShortcut(target); err != nil {
		step("shortcut", "fail", err.Error())
	} else {
		step("shortcut", "ok", "")
	}

	emit("window.done()")
}

func main() {
	app = webview.New(false)
	defer app.Destroy()
	app.SetTitle("DeepSeek Harness Setup")
	app.SetSize(640, 740, webview.HintNone)
	app.SetHtml(uiHTML)

	// --auto [pt|en]: skip the language screen, useful for silent installs and testing
	if len(os.Args) > 1 {
		lang := "en"
		for _, a := range os.Args[1:] {
			if strings.HasPrefix(a, "pt") {
				lang = "pt"
			}
		}
		go func() {
			time.Sleep(700 * time.Millisecond)
			emit(fmt.Sprintf("window.autoStart(%s)", jsonStr(lang)))
		}()
	}

	app.Bind("startInstall", func(opts string) string {
		go installAll(opts)
		return "ok"
	})
	app.Bind("openApp", func() string {
		target := filepath.Join(localAppData(), "Programs", "DeepSeekHarness", exeName)
		cmd := exec.Command(target)
		cmd.Dir = filepath.Dir(target)
		_ = cmd.Start()
		return "ok"
	})
	app.Bind("quitApp", func() string {
		app.Terminate()
		return "ok"
	})

	app.Run()
}
