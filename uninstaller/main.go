package main

import (
	_ "embed"
	"encoding/json"
	"io"
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
	appName         = "DeepSeek Harness"
	exeName         = "DeepSeek Harness.exe"
	programDir      = `C:\Program Files\DeepSeek Harness`
	uninstallRegKey = `HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\DeepSeekHarness`
)

var (
	app      webview.WebView
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSendMessageW         = user32.NewProc("SendMessageW")
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
				procSendMessageW.Call(iconHWND, wmSetIcon, iconBig, big)
			}
			if small, _, _ := procLoadImageW.Call(inst, 1, imageIcon, 16, 16, 0); small != 0 {
				procSendMessageW.Call(iconHWND, wmSetIcon, iconSmall, small)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func emit(js string) { app.Dispatch(func() { app.Eval(js) }) }

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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

type state struct {
	App      bool `json:"app"`
	Dsh      bool `json:"dsh"`
	Bridge   bool `json:"bridge"`
	Opencode bool `json:"opencode"`
	Node     bool `json:"node"`
	Data     bool `json:"data"`
}

func bridgeInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(home, ".dsh", "profiles", "web", "package.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "dsh-opencode-bridge")
}

func appDirs() []string {
	dirs := []string{programDir}
	if perUser := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "DeepSeekHarness"); perUser != programDir {
		dirs = append(dirs, perUser)
	}
	return dirs
}

func installedDirs() []string {
	var found []string
	for _, d := range appDirs() {
		if _, err := os.Stat(filepath.Join(d, exeName)); err == nil {
			found = append(found, d)
		}
	}
	return found
}

func scan() string {
	home, _ := os.UserHomeDir()
	s := state{
		App:      len(installedDirs()) > 0,
		Dsh:      has("dsh"),
		Bridge:   bridgeInstalled(),
		Opencode: has("opencode"),
		Node:     has("node"),
		Data:     false,
	}
	if _, err := os.Stat(filepath.Join(home, ".dsh")); err == nil {
		s.Data = true
	}
	b, _ := json.Marshal(s)
	return string(b)
}

func removeShortcuts() {
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	common := filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs")
	for _, p := range []string{filepath.Join(desktop, appName+".lnk"), filepath.Join(common, appName+".lnk")} {
		_ = os.Remove(p)
	}
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

func removeApp() error {
	_ = exec.Command("taskkill", "/IM", exeName, "/F").Run()
	removeShortcuts()
	_, _ = runHidden("reg", "delete", uninstallRegKey, "/f")

	// deleting by path beats any cmd quoting trick
	var stuck []string
	for _, dir := range installedDirs() {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			stuck = append(stuck, dir)
		}
	}
	if len(stuck) == 0 {
		return nil
	}

	// this exe is inside one of those folders: relaunch from %TEMP%, then the
	// copy deletes the folder after this process is gone
	self, err := os.Executable()
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), "DeepSeekHarness-Uninstall-cleanup.exe")
	if err := copyFile(self, tmp); err != nil {
		return err
	}
	cmd := exec.Command(tmp, append([]string{"--cleanup"}, stuck...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
	return cmd.Start()
}

func npmUninstall(pkg string) error {
	npm := "npm.cmd"
	if !has(npm) {
		npm = "npm"
	}
	_, err := runHidden(npm, "uninstall", "-g", pkg)
	return err
}

func uninstallAll(selJSON string) {
	var sel state
	_ = json.Unmarshal([]byte(selJSON), &sel)

	if sel.Bridge {
		emit(`window.step('bridge','run')`)
		if !has("dsh") {
			emit(`window.step('bridge','ok')`)
		} else if _, err := runHidden("cmd.exe", "/c", "dsh", "plugin", "--profile", "web", "remove", "dsh-opencode-bridge"); err != nil {
			emit(`window.step('bridge','fail',` + jsonStr(err.Error()) + `)`)
		} else {
			emit(`window.step('bridge','ok')`)
		}
	}
	if sel.Dsh {
		emit(`window.step('dsh','run')`)
		if err := npmUninstall("@deepseek-ai/dsh"); err != nil {
			emit(`window.step('dsh','fail',` + jsonStr(err.Error()) + `)`)
		} else {
			emit(`window.step('dsh','ok')`)
		}
	}
	if sel.Opencode {
		emit(`window.step('opencode','run')`)
		if err := npmUninstall("@opencode/cli"); err != nil {
			emit(`window.step('opencode','fail',` + jsonStr(err.Error()) + `)`)
		} else {
			emit(`window.step('opencode','ok')`)
		}
	}
	if sel.Node {
		emit(`window.step('node','run')`)
		if _, err := runHidden("winget", "uninstall", "--id", "OpenJS.NodeJS.LTS", "-e", "--silent"); err != nil {
			emit(`window.step('node','fail',` + jsonStr(err.Error()) + `)`)
		} else {
			emit(`window.step('node','ok')`)
		}
	}
	if sel.Data {
		emit(`window.step('data','run')`)
		home, _ := os.UserHomeDir()
		if err := os.RemoveAll(filepath.Join(home, ".dsh")); err != nil {
			emit(`window.step('data','fail',` + jsonStr(err.Error()) + `)`)
		} else {
			emit(`window.step('data','ok')`)
		}
	}
	if sel.App {
		emit(`window.step('app','run')`)
		if err := removeApp(); err != nil {
			emit(`window.step('app','fail',` + jsonStr(err.Error()) + `)`)
		} else {
			emit(`window.step('app','ok')`)
		}
	}

	emit(`window.done()`)
	time.Sleep(900 * time.Millisecond)
	app.Dispatch(func() { app.Terminate() })
}

func main() {
	// cleanup pass launched from %TEMP% after the real uninstaller exits
	if len(os.Args) > 1 && os.Args[1] == "--cleanup" {
		time.Sleep(1200 * time.Millisecond)
		for _, dir := range os.Args[2:] {
			for i := 0; i < 8; i++ {
				if err := os.RemoveAll(dir); err == nil || os.IsNotExist(err) {
					break
				}
				time.Sleep(400 * time.Millisecond)
			}
		}
		return
	}

	app = webview.New(false)
	defer app.Destroy()
	app.SetTitle("DeepSeek Harness Uninstall")
	app.SetSize(640, 700, webview.HintNone)
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
			emit("pick(" + jsonStr(lang) + ")")
		}()
	}

	app.Bind("scan", func() string { return scan() })
	app.Bind("uninstall", func(sel string) string {
		go uninstallAll(sel)
		return "ok"
	})
	app.Bind("quitApp", func() string {
		app.Terminate()
		return "ok"
	})

	app.Run()
}
