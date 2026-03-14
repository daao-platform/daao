//go:build windows
// +build windows

package pty

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsPty struct {
	inputPipe  *os.File // we write here to send input to the child
	outputPipe *os.File // we read here to receive output from the child
	hpcon      windows.Handle
	closeOnce  sync.Once
}

// COORD represents a coordinate in the console (X, Y)
type COORD struct {
	X int16
	Y int16
}

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procCreatePseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
)

// Start starts a process with the ConPTY pseudo console
func (p *windowsPty) Start(binary string, args []string, dir string, env []string, detachFlags uint32) (*os.Process, error) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	cmdExe := filepath.Join(systemRoot, `System32\cmd.exe`)

	// Resolve full path to binary.
	// If the caller already resolved an absolute path (e.g. via resolveBinary),
	// skip exec.LookPath — it only searches the system PATH which may not
	// contain user-installed tool directories when running as a service.
	var fullPath string
	if filepath.IsAbs(binary) {
		if _, err := os.Stat(binary); err != nil {
			return nil, fmt.Errorf("binary not found at absolute path %s: %w", binary, err)
		}
		fullPath = binary
	} else {
		var err error
		fullPath, err = exec.LookPath(binary)
		if err != nil {
			if binary == "cmd.exe" || binary == "cmd" {
				fullPath = cmdExe
				if _, err := os.Stat(fullPath); err != nil {
					return nil, fmt.Errorf("failed to find binary %s and fallback failed: %w", binary, err)
				}
			} else {
				return nil, fmt.Errorf("failed to find binary %s: %w", binary, err)
			}
		}
	}

	// Normalize path — remove double backslashes
	fullPath = filepath.Clean(fullPath)

	// .cmd and .bat scripts cannot be exec'd directly via CreateProcess;
	// wrap them through cmd.exe /c.
	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext == ".cmd" || ext == ".bat" {
		scriptPath := fullPath
		fullPath = cmdExe
		args = append([]string{cmdExe, "/c", scriptPath}, args[1:]...)
	}

	// Build command line: quote args that contain spaces
	quotedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t") {
			quotedArgs = append(quotedArgs, `"`+arg+`"`)
		} else {
			quotedArgs = append(quotedArgs, arg)
		}
	}
	cmdLine := strings.Join(quotedArgs, " ")

	// Setup Attribute List using x/sys/windows helpers
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("failed to create attribute list: %w", err)
	}
	defer attrList.Delete()

	// CRITICAL: PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE expects lpValue to BE the
	// HPCON handle itself (cast to a pointer), NOT a pointer to the handle.
	// Passing &p.hpcon gives Windows the Go variable's memory address, which
	// it silently rejects, causing a fallback to a visible console window.
	// CRITICAL: PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE expects lpValue to BE the
	// HPCON handle itself (cast to a pointer), NOT a pointer to the handle.
	// Passing &p.hpcon gives Windows the Go variable's memory address, which
	// it silently rejects, causing a fallback to a visible console window.
	// go vet flags this as "possible misuse of unsafe.Pointer" but this is a
	// false positive: hpcon is a Windows kernel handle, not a GC-managed pointer.
	err = attrList.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(uintptr(p.hpcon)), unsafe.Sizeof(p.hpcon)) //nolint:govet
	if err != nil {
		return nil, fmt.Errorf("failed to update attribute list: %w", err)
	}

	si := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:    uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags: windows.STARTF_USESTDHANDLES, // Prevent legacy visible-console fallback
		},
		ProcThreadAttributeList: attrList.List(),
	}

	pi := new(windows.ProcessInformation)

	binaryPtr, _ := windows.UTF16PtrFromString(fullPath)
	cmdLinePtr, _ := windows.UTF16PtrFromString(cmdLine)

	// Pass nil for lpCurrentDirectory so the child inherits our working dir;
	// an empty string is invalid and causes CreateProcess to fail.
	var dirPtr *uint16
	if dir != "" {
		dirPtr, _ = windows.UTF16PtrFromString(dir)
	}

	var envPtr *uint16
	if len(env) > 0 {
		var buf []uint16
		for _, e := range env {
			u, _ := windows.UTF16FromString(e)
			buf = append(buf, u...) // includes null terminator for each var
		}
		buf = append(buf, 0) // double-null terminator for the block
		envPtr = &buf[0]
	}

	// CREATE_UNICODE_ENVIRONMENT (0x400) is required when lpEnvironment is a
	// UTF-16 block; without it Windows treats the block as ANSI and the call
	// returns ERROR_INVALID_PARAMETER.
	const createUnicodeEnvironment uint32 = 0x00000400
	creationFlags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT) | detachFlags | createUnicodeEnvironment
	log.Printf("ConPTY Start: Creating process %s with cmdLine [%s] and flags %x", fullPath, cmdLine, creationFlags)

	// In Go, passing a pointer to the nested StartupInfo struct is the correct way to pass an extended struct
	// to a function that expects the base struct, because the layout guarantees the base struct is first.
	err = windows.CreateProcess(
		binaryPtr,
		cmdLinePtr,
		nil,
		nil,
		false,
		creationFlags,
		envPtr,
		dirPtr,
		&si.StartupInfo,
		pi,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create process: %w", err)
	}

	windows.CloseHandle(pi.Thread)

	// Do NOT close pi.Process — os.FindProcess on Windows opens its own handle
	// via OpenProcess, but we already have a valid handle from CreateProcess.
	// We pass it directly so Process.Wait() can use it.
	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		windows.CloseHandle(pi.Process)
		return nil, fmt.Errorf("failed to find process: %w", err)
	}
	// Note: os.FindProcess on Windows calls OpenProcess internally, so it has
	// its own handle. We can safely close the CreateProcess handle now.
	windows.CloseHandle(pi.Process)

	return proc, nil
}

// CreatePseudoConsole creates a new ConPTY pseudo console.
// COORD must be passed by value (packed into the low 32 bits of a uintptr),
// NOT as a pointer — that is the x64 calling convention for 4-byte structs.
//
// HANDLE SAFETY: This function accepts raw windows.Handle values rather than
// *os.File to avoid os.File.Fd() which Go 1.12+ warns may put the descriptor
// in blocking mode and the handle becomes invalid after os.File is GC'd.
// Callers must ensure handles remain valid for the duration of this call.
func CreatePseudoConsole(size COORD, inputHandle, outputHandle windows.Handle, flags uint32) (windows.Handle, error) {
	var hpcon windows.Handle

	// Pack COORD {X int16, Y int16} into low 32 bits: X in bits 0-15, Y in bits 16-31.
	// Cast through uint16 first to avoid sign-extension of negative int16 values.
	coordVal := uintptr(uint32(uint16(size.X)) | uint32(uint16(size.Y))<<16)

	ret, _, err := procCreatePseudoConsole.Call(
		coordVal,
		uintptr(inputHandle),
		uintptr(outputHandle),
		uintptr(flags),
		uintptr(unsafe.Pointer(&hpcon)),
	)

	if ret != 0 {
		return 0, err
	}
	return hpcon, nil
}

// ResizePseudoConsole resizes the ConPTY pseudo console.
// COORD must be passed by value, same as CreatePseudoConsole.
func ResizePseudoConsole(hpcon windows.Handle, size COORD) error {
	coordVal := uintptr(uint32(uint16(size.X)) | uint32(uint16(size.Y))<<16)
	ret, _, err := procResizePseudoConsole.Call(uintptr(hpcon), coordVal)
	if ret != 0 {
		return err
	}
	return nil
}

// ClosePseudoConsole closes the ConPTY pseudo console
func ClosePseudoConsole(hpcon windows.Handle) error {
	ret, _, err := procClosePseudoConsole.Call(uintptr(hpcon))
	if ret != 0 {
		return err
	}
	return nil
}

// NewPty creates a new PTY using Windows ConPTY.
//
// HANDLE SAFETY: We create two pipe pairs via CreatePipe, which returns raw
// syscall.Handle values. Only the handles we perform I/O on (inputWrite,
// outputRead) are wrapped in os.File. The handles passed to
// CreatePseudoConsole (inputRead, outputWrite) remain raw and are closed
// with CloseHandle after ConPTY has duplicated them internally. This avoids
// the os.File.Fd() GC-safety issue entirely.
func NewPty(cols, rows uint16) (Pty, error) {
	// inputRead: ConPTY reads from this (raw handle). inputWrite: We write to this (os.File).
	inputReadHandle, inputWriteHandle, err := CreatePipeRaw()
	if err != nil {
		return nil, err
	}

	// outputRead: We read from this (os.File). outputWrite: ConPTY writes to this (raw handle).
	outputReadHandle, outputWriteHandle, err := CreatePipeRaw()
	if err != nil {
		syscall.CloseHandle(inputReadHandle)
		syscall.CloseHandle(inputWriteHandle)
		return nil, err
	}

	// Handles passed to CreatePseudoConsole MUST NOT be inherited.
	SetHandleInheritMode(inputReadHandle)
	SetHandleInheritMode(outputWriteHandle)

	size := COORD{X: int16(cols), Y: int16(rows)}
	hpcon, err := CreatePseudoConsole(size, windows.Handle(inputReadHandle), windows.Handle(outputWriteHandle), 0)

	// Per Microsoft docs, the handles given to CreatePseudoConsole are "consumed"
	// — ConPTY has duplicated them internally. Close them now so that when the
	// child process exits ConPTY's dup is the last write handle on outputWrite,
	// and outputRead.Read() correctly returns EOF on child exit.
	syscall.CloseHandle(inputReadHandle)
	syscall.CloseHandle(outputWriteHandle)

	if err != nil {
		syscall.CloseHandle(inputWriteHandle)
		syscall.CloseHandle(outputReadHandle)
		return nil, err
	}

	// Only wrap the I/O handles in os.File — these are the ones we Read/Write on.
	// os.File takes ownership and will CloseHandle when closed.
	inputWrite := os.NewFile(uintptr(inputWriteHandle), "conpty-input-write")
	outputRead := os.NewFile(uintptr(outputReadHandle), "conpty-output-read")

	pty := &windowsPty{
		inputPipe:  inputWrite,
		outputPipe: outputRead,
		hpcon:      hpcon,
	}

	return pty, nil
}

// Resize resizes the PTY
func (p *windowsPty) Resize(cols, rows uint16) error {
	size := COORD{X: int16(cols), Y: int16(rows)}
	return ResizePseudoConsole(p.hpcon, size)
}

// Read reads from the PTY output
func (p *windowsPty) Read(b []byte) (int, error) {
	return p.outputPipe.Read(b)
}

// Write writes to the PTY input
func (p *windowsPty) Write(b []byte) (int, error) {
	return p.inputPipe.Write(b)
}

// SetReadDeadline sets the read deadline for the PTY.
func (p *windowsPty) SetReadDeadline(t time.Time) error {
	err := p.outputPipe.SetReadDeadline(t)
	runtime.KeepAlive(p.outputPipe) // prevent GC from closing the handle during the syscall
	return err
}

// Close closes the PTY and releases resources. Safe to call multiple times.
// Closing hpcon causes ConPTY to release its internal pipe handles, which
// will make any blocked outputPipe.Read() return an error.
func (p *windowsPty) Close() error {
	var firstErr error
	p.closeOnce.Do(func() {
		if p.hpcon != 0 {
			if err := ClosePseudoConsole(p.hpcon); err != nil && firstErr == nil {
				firstErr = err
			}
			p.hpcon = 0
		}
		if p.inputPipe != nil {
			if err := p.inputPipe.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			p.inputPipe = nil
		}
		if p.outputPipe != nil {
			if err := p.outputPipe.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			p.outputPipe = nil
		}
	})
	return firstErr
}

// Fd returns the output pipe's file descriptor.
// WARNING: The returned uintptr is only valid for the lifetime of the os.File.
// Callers must use runtime.KeepAlive(p) to prevent GC from closing the handle.
func (p *windowsPty) Fd() uintptr {
	fd := p.outputPipe.Fd()
	runtime.KeepAlive(p.outputPipe)
	return fd
}

// CreatePipeRaw creates a Windows pipe pair and returns raw syscall.Handle values.
// This avoids wrapping in os.File (and the Fd() GC-safety issue) for handles
// that are only passed to Windows APIs and never used for Go I/O.
// The caller is responsible for calling syscall.CloseHandle on handles that are
// not later wrapped in os.File.
func CreatePipeRaw() (readHandle, writeHandle syscall.Handle, err error) {
	var sa syscall.SecurityAttributes
	sa.Length = uint32(unsafe.Sizeof(sa))
	sa.InheritHandle = 1

	err = syscall.CreatePipe(&readHandle, &writeHandle, &sa, 0)
	if err != nil {
		return syscall.InvalidHandle, syscall.InvalidHandle, err
	}
	return readHandle, writeHandle, nil
}

// SetHandleInheritMode clears the HANDLE_FLAG_INHERIT bit on a raw handle.
// This is used for handles passed to CreatePseudoConsole, which must not be
// inherited by child processes.
func SetHandleInheritMode(h syscall.Handle) error {
	return syscall.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0)
}
