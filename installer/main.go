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
	"unsafe"

	webview "github.com/webview/webview_go"
)

//go:embed ui.html
var uiHTML string

const (
	repoURL         = "https://github.com/Ryanabcraft/deepseek-harness-pc"
	assetURL        = repoURL + "/releases/latest/download/DeepSeek-Harness.exe"
	uninstallURL    = repoURL + "/releases/latest/download/DeepSeek-Harness-Uninstall.exe"
	appName         = "DeepSeek Harness"
	exeName         = "DeepSeek Harness.exe"
	uninstallerName = "DeepSeek-Harness-Uninstall.exe"
	bridgePkg       = "dsh-opencode-bridge@0.5.2"
	profileName     = "web"
)

var (
	app      webview.WebView
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetWindowIcon        = user32.NewProc("SendMessageW")
	procLoadImageW           = user32.NewProc("LoadImageW")
	procEnumWindows          = user32.NewProc("EnumWindows")
	procGetWindowThreadPID   = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmSetIcon = 0x0080
	iconSmall = 0
	iconBig   = 1
	imageIcon = 1
)

var (
	iconTargetPID uint32
	iconHWND      uintptr
)

func enumWindowCallback(hwnd uintptr, _ uintptr) uintptr {
	var pid uint32
	procGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != iconTargetPID {
		return 1
	}
	if vis, _, _ := procIsWindowVisible.Call(hwnd); vis == 0 {
		return 1
	}
	if n, _, _ := procGetWindowTextLengthW.Call(hwnd); n == 0 {
		return 1
	}
	iconHWND = hwnd
	return 0
}

func setWindowIcon() {
	iconTargetPID = uint32(os.Getpid())
	cb := syscall.NewCallback(enumWindowCallback)
	inst, _, _ := procGetModuleHandleW.Call(0)
	for i := 0; i < 120; i++ {
		iconHWND = 0
		procEnumWindows.Call(cb, 0)
		if iconHWND != 0 {
			if big, _, _ := procLoadImageW.Call(inst, 1, imageIcon, 32, 32, 0); big != 0 {
				procSetWindowIcon.Call(iconHWND, wmSetIcon, iconBig, big)
			}
			if small, _, _ := procLoadImageW.Call(inst, 1, imageIcon, 16, 16, 0); small != 0 {
				procSetWindowIcon.Call(iconHWND, wmSetIcon, iconSmall, small)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

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
	ps := `[Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')`
	out, err := runHidden("powershell", "-NoProfile", "-Command", ps)
	if err == nil && strings.TrimSpace(out) != "" {
		os.Setenv("PATH", strings.TrimSpace(out))
	}
}

func localAppData() string { return os.Getenv("LOCALAPPDATA") }

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ---------------------------------------------------------------- install steps

func ensureNode() (string, string, error) {
	if has("node") {
		out, _ := runHidden("node", "--version")
		return "ok", "present:" + out, nil
	}
	step("node", "run", "")
	if !has("winget") {
		return "fail", "", fmt.Errorf("winget not available")
	}
	runHidden("winget", "install", "--id", "OpenJS.NodeJS.LTS", "-e", "--silent",
		"--accept-package-agreements", "--accept-source-agreements")
	refreshPath()
	if !has("node") {
		if _, statErr := os.Stat(`C:\Program Files\nodejs\node.exe`); statErr == nil {
			os.Setenv("PATH", `C:\Program Files\nodejs;`+os.Getenv("PATH"))
		}
	}
	if !has("node") {
		return "fail", "", fmt.Errorf("node not on PATH after install")
	}
	out, _ := runHidden("node", "--version")
	return "ok", "installed:" + out, nil
}

func npmCmd() string {
	if has("npm.cmd") {
		return "npm.cmd"
	}
	return "npm"
}

func ensureDsh() (string, string, error) {
	if has("dsh") {
		return "ok", "present:", nil
	}
	step("dsh", "run", "")
	if _, err := runHidden(npmCmd(), "install", "-g", "@deepseek-ai/dsh"); err != nil {
		return "fail", "", err
	}
	refreshPath()
	if !has("dsh") {
		return "fail", "", fmt.Errorf("dsh not on PATH after install")
	}
	return "ok", "installed:", nil
}

func profileManifest() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dsh", "profiles", profileName, "package.json")
}

func bridgeInstalled() bool {
	data, err := os.ReadFile(profileManifest())
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "dsh-opencode-bridge")
}

// ensureBridge installs the DSH plugin that exposes the free OpenCode models.
// Without it the opencode-bridge provider does not exist, so the app has no model.
func ensureBridge() (string, string, error) {
	if bridgeInstalled() {
		return "ok", "present:", nil
	}
	step("bridge", "run", "")
	if _, err := runHidden("cmd.exe", "/c", "dsh", "plugin", "--profile", profileName, "add", bridgePkg); err != nil {
		return "fail", "", err
	}
	if !bridgeInstalled() {
		return "fail", "", fmt.Errorf("bridge not in profile after install")
	}
	return "ok", "installed:", nil
}

func ensureOpencode(want bool) (string, string, error) {
	if has("opencode") {
		return "ok", "present:", nil
	}
	if !want {
		return "skip", "", nil
	}
	step("opencode", "run", "")
	if _, err := runHidden(npmCmd(), "install", "-g", "@opencode/cli"); err != nil {
		return "fail", "", err
	}
	return "ok", "installed:", nil
}

// ---------------------------------------------------------------- app placement

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

func installScript(target string) string {
	dir := filepath.Dir(target)
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	common := filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs")
	uninst := filepath.Join(dir, uninstallerName)
	return fmt.Sprintf(`
$target = %s
$dir = %s
$uninst = %s
$icon = $target + ',0'
$shell = New-Object -ComObject WScript.Shell
foreach ($where in @(%s, %s)) {
  if (-not (Test-Path $where)) { continue }
  $lnk = Join-Path $where %s
  $s = $shell.CreateShortcut($lnk)
  $s.TargetPath = $target
  $s.WorkingDirectory = $dir
  $s.IconLocation = $icon
  $s.Description = 'DeepSeek Harness'
  $s.Save()
}
$key = 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\DeepSeekHarness'
New-Item -Path $key -Force | Out-Null
Set-ItemProperty -Path $key -Name DisplayName -Value 'DeepSeek Harness'
Set-ItemProperty -Path $key -Name DisplayVersion -Value '1.5.2'
Set-ItemProperty -Path $key -Name Publisher -Value 'Ryanabcraft'
Set-ItemProperty -Path $key -Name DisplayIcon -Value $target
Set-ItemProperty -Path $key -Name InstallLocation -Value $dir
Set-ItemProperty -Path $key -Name UninstallString -Value $uninst
Set-ItemProperty -Path $key -Name NoModify -Value 1 -Type DWord
Set-ItemProperty -Path $key -Name NoRepair -Value 1 -Type DWord
`, psQuote(target), psQuote(dir), psQuote(uninst),
		psQuote(desktop), psQuote(common), psQuote(appName+".lnk"))
}

// placeApp installs into C:\Program Files. The setup carries a
// requireAdministrator manifest, so the copy needs no extra elevation prompt.
// DSH_SETUP_DIR exists for tests and per-user installs.
func placeApp(tmpApp, tmpUn string) (string, error) {
	dir := `C:\Program Files\DeepSeek Harness`
	if custom := os.Getenv("DSH_SETUP_DIR"); custom != "" {
		dir = custom
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dir = filepath.Join(localAppData(), "Programs", "DeepSeekHarness")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	target := filepath.Join(dir, exeName)
	if err := copyFile(tmpApp, target); err != nil {
		return "", err
	}
	if tmpUn != "" {
		_ = copyFile(tmpUn, filepath.Join(dir, uninstallerName))
	}
	_, _ = runHidden("powershell", "-NoProfile", "-Command", installScript(target))
	return target, nil
}

// ---------------------------------------------------------------- orchestration

func installAll(optsJSON string) {
	var opts struct {
		Opencode bool   `json:"opencode"`
		Lang     string `json:"lang"`
	}
	_ = json.Unmarshal([]byte(optsJSON), &opts)

	if state, detail, err := ensureNode(); err != nil {
		step("node", "fail", err.Error())
		emit("window.fail(" + jsonStr("node") + ")")
		return
	} else {
		step("node", state, detail)
	}

	if state, detail, err := ensureDsh(); err != nil {
		step("dsh", "fail", err.Error())
		emit("window.fail(" + jsonStr("dsh") + ")")
		return
	} else {
		step("dsh", state, detail)
	}

	if state, detail, err := ensureBridge(); err != nil {
		step("bridge", "fail", err.Error())
		emit("window.fail(" + jsonStr("bridge") + ")")
		return
	} else {
		step("bridge", state, detail)
	}

	if state, detail, err := ensureOpencode(opts.Opencode); err != nil {
		step("opencode", "fail", err.Error())
		emit("window.fail(" + jsonStr("opencode") + ")")
		return
	} else {
		step("opencode", state, detail)
	}

	step("app", "run", "")
	tmp := filepath.Join(os.TempDir(), "DeepSeekHarness-download.exe")
	if err := download(assetURL, tmp); err != nil {
		step("app", "fail", err.Error())
		emit("window.fail(" + jsonStr("app") + ")")
		return
	}
	tmpUn := filepath.Join(os.TempDir(), "DeepSeekHarness-uninstall.exe")
	if err := download(uninstallURL, tmpUn); err != nil {
		tmpUn = ""
	}
	target, err := placeApp(tmp, tmpUn)
	os.Remove(tmp)
	os.Remove(tmpUn)
	if err != nil {
		step("app", "fail", err.Error())
		emit("window.fail(" + jsonStr("app") + ")")
		return
	}
	curTarget = target
	step("app", "ok", "installed:")
	step("shortcut", "ok", "installed:")

	emit("window.done()")
}

var curTarget string

func main() {
	app = webview.New(false)
	defer app.Destroy()
	app.SetTitle("DeepSeek Harness Setup")
	app.SetSize(640, 740, webview.HintNone)
	app.SetHtml(uiHTML)
	go setWindowIcon()

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
		target := curTarget
		if target == "" {
			target = filepath.Join(`C:\Program Files\DeepSeek Harness`, exeName)
		}
		cmd := exec.Command(target)
		cmd.Dir = filepath.Dir(target)
		_ = cmd.Start()
		go func() {
			time.Sleep(900 * time.Millisecond)
			app.Dispatch(func() { app.Terminate() })
		}()
		return "ok"
	})
	app.Bind("quitApp", func() string {
		app.Terminate()
		return "ok"
	})

	app.Run()
}
