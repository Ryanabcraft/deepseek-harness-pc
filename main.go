package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	webview "github.com/webview/webview_go"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")

	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procLoadImageW               = user32.NewProc("LoadImageW")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
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
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
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

// webview creates its own window class carrying the default icon, so the title
// bar shows a generic glyph. Find our own top-level window by process id, then
// push the executable's icon resource (id 1) into WM_SETICON for both sizes.
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

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x2000
	processSetQuotaAndTerminate       = 0x0101
)

type jobBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobExtendedLimitInformation struct {
	BasicLimitInformation jobBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// jobHandle stays open for the process lifetime: when this exe dies for any
// reason (X button, crash, Task Manager), Windows closes the job and kills the
// dsh tree with it, so no orphan node keeps holding a port.
var jobHandle uintptr

func bindToJob(pid int) {
	h, _, _ := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		return
	}
	var info jobExtendedLimitInformation
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	if ok, _, _ := procSetInformationJobObject.Call(h, jobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info)); ok == 0 {
		procCloseHandle.Call(h)
		return
	}
	ph, _, _ := procOpenProcess.Call(processSetQuotaAndTerminate, 0, uintptr(pid))
	if ph == 0 {
		procCloseHandle.Call(h)
		return
	}
	procAssignProcessToJobObject.Call(h, ph)
	procCloseHandle.Call(ph)
	jobHandle = h
}

func messageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10)
}

const splashHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>DeepSeek Harness</title>
<style>
  html,body{height:100%;margin:0;background:#0b0d13;overflow:hidden;
    font-family:"Cascadia Mono",Consolas,monospace;color:#dde4f2}
  .wrap{height:100%;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:26px}
  .logo{white-space:pre;font-size:15px;line-height:1;font-weight:700;
    background:linear-gradient(180deg,#4d6bfe 0%,#7df9ff 100%);
    -webkit-background-clip:text;background-clip:text;color:transparent}
  .title{letter-spacing:.42em;font-size:13px;color:#dde4f2;text-indent:.42em}
  .rule{width:520px;height:1px;background:linear-gradient(90deg,transparent,#2a3350,transparent)}
  .sub{font-size:11px;letter-spacing:.18em;color:#6f7a99;text-transform:uppercase}
  .bar{width:280px;height:3px;border-radius:3px;background:#161b2a;overflow:hidden;margin-top:10px}
  .bar i{display:block;height:100%;width:38%;border-radius:3px;
    background:linear-gradient(90deg,#4d6bfe,#7df9ff);animation:slide 1.25s ease-in-out infinite}
  @keyframes slide{0%{transform:translateX(-100%)}100%{transform:translateX(360%)}}
  .dot{display:inline-block;width:6px;height:6px;border-radius:50%;background:#56d6a3;
    margin-right:8px;animation:pulse 1.4s ease-in-out infinite}
  @keyframes pulse{0%,100%{opacity:.25}50%{opacity:1}}
</style></head><body>
<div class="wrap">
  <div class="logo">   ██████╗ ██████╗ ███████╗██╗  ██╗
   ██╔══██╗██╔══██╗██╔════╝██║ ██╔╝
   ██║  ██║██████╔╝███████╗█████╔╝
   ██║  ██║██╔═══╝ ╚════██║██╔═██╗
   ██████╔╝██║     ███████║██║  ██╗
   ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═╝</div>
  <div class="title">DEEPSEEK HARNESS</div>
  <div class="rule"></div>
  <div class="bar"><i></i></div>
  <div class="sub" style="letter-spacing:.09em;text-transform:none;color:#7c88a8">
    <span class="dot"></span>booting harness</div>
</div></body></html>`

func portFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func findDsh() (string, error) {
	if root := os.Getenv("APPDATA"); root != "" {
		candidate := filepath.Join(root, "npm", "dsh.cmd")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	for _, name := range []string{"dsh.cmd", "dsh.ps1", "dsh"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("dsh nao encontrado no PATH")
}

func killTree(pid int) {
	exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

func userPickedPort(args []string) bool {
	for _, a := range args {
		if a == "--port" || strings.HasPrefix(a, "--port=") {
			return true
		}
	}
	return false
}

func main() {
	dsh, err := findDsh()
	if err != nil {
		messageBox("DeepSeek Harness", "dsh nao encontrado.\n\nInstale com:\n  npm i -g @deepseek-ai/dsh")
		return
	}

	args := append([]string{"web", "--no-open"}, os.Args[1:]...)
	profile := profileFromArgs(os.Args[1:])

	// window first: the splash covers the model check plus the dsh boot
	app := webview.New(false)
	app.SetTitle("DeepSeek Harness")
	app.SetSize(1460, 920, webview.HintNone)
	app.SetHtml(splashHTML)
	go setWindowIcon()

	urlCh := make(chan string, 1)
	doneCh := make(chan struct{})
	var gotURL atomic.Bool
	var bootErr atomic.Value
	var stderr strings.Builder

	go func() {
		defer close(doneCh)
		ensureDefaultModel(profile)

		if !userPickedPort(os.Args[1:]) && !portFree(3080) {
			args = append(args, "--port", "0")
		}
		target := dsh
		if strings.ContainsAny(target, " \t") {
			target = `"` + target + `"`
		}

		cmd := exec.Command("cmd.exe", append([]string{"/c", target}, args...)...)
		if home, err := os.UserHomeDir(); err == nil {
			cmd.Dir = home
		}
		// no console window for cmd.exe or the node child: the app owns the UI
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			bootErr.Store("falha ao capturar o stdout do dsh:\n" + err.Error())
			return
		}
		cmd.Stderr = &stderr
		cmd.Env = os.Environ()

		if err := cmd.Start(); err != nil {
			bootErr.Store("falha ao iniciar o dsh:\n" + err.Error())
			return
		}
		bindToJob(cmd.Process.Pid)
		defer killTree(cmd.Process.Pid)

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			if i := strings.Index(line, "http://127.0.0.1:"); i >= 0 {
				select {
				case urlCh <- strings.TrimSpace(line[i:]):
				default:
				}
			}
		}
	}()

	go func() {
		select {
		case u := <-urlCh:
			gotURL.Store(true)
			time.Sleep(350 * time.Millisecond) // let the splash land before swapping
			app.Dispatch(func() { app.Navigate(u) })
		case <-doneCh:
			app.Dispatch(func() { app.Terminate() })
		case <-time.After(120 * time.Second):
			app.Dispatch(func() { app.Terminate() })
		}
	}()

	app.Run()
	app.Destroy()

	if !gotURL.Load() {
		msg := "O harness nao chegou a publicar a URL."
		if v := bootErr.Load(); v != nil {
			msg = v.(string)
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			if len(s) > 1200 {
				s = s[len(s)-1200:]
			}
			msg += "\n\n" + s
		}
		messageBox("DeepSeek Harness", msg)
	}
}
