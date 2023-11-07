package lua

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	humanize "github.com/dustin/go-humanize"
	lua "github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

type State struct {
	L *lua.LState
}

func NewState() *State {
	return &State{
		L: lua.NewState(),
	}
}

// LoadFile loads a lua file
func (s *State) LoadFile(module string, file string, data []byte) error {
	pluginDef := []byte("module(\"" + module + "\", package.seeall)")

	if fn, err := s.L.Load(bytes.NewReader(append(pluginDef, data...)), file); err != nil {
		return err
	} else {
		s.L.Push(fn)
		return s.L.PCall(0, lua.MultRet, nil)
	}
}

// Import allows a lua plugin to import a package
func (s *State) Import(pkg string) *lua.LTable {
	switch pkg {
	case "fmt":
		return s.importFmt()
	case "io":
		return s.importIo()
	case "io/ioutil", "ioutil":
		return s.importIoUtil()
	case "net":
		return s.importNet()
	case "math":
		return s.importMath()
	case "math/rand":
		return s.importMathRand()
	case "os":
		return s.importOs()
	case "runtime":
		return s.importRuntime()
	case "path":
		return s.importPath()
	case "path/filepath", "filepath":
		return s.importFilePath()
	case "strings":
		return s.importStrings()
	case "regexp":
		return s.importRegexp()
	case "errors":
		return s.importErrors()
	case "time":
		return s.importTime()
	case "unicode/utf8", "utf8":
		return s.importUtf8()
	case "humanize":
		return s.importHumanize()
	default:
		return nil
	}
}

func (s *State) importFmt() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Errorf", luar.New(s.L, fmt.Errorf))
	s.L.SetField(pkg, "Fprint", luar.New(s.L, fmt.Fprint))
	s.L.SetField(pkg, "Fprintf", luar.New(s.L, fmt.Fprintf))
	s.L.SetField(pkg, "Fprintln", luar.New(s.L, fmt.Fprintln))
	s.L.SetField(pkg, "Fscan", luar.New(s.L, fmt.Fscan))
	s.L.SetField(pkg, "Fscanf", luar.New(s.L, fmt.Fscanf))
	s.L.SetField(pkg, "Fscanln", luar.New(s.L, fmt.Fscanln))
	s.L.SetField(pkg, "Print", luar.New(s.L, fmt.Print))
	s.L.SetField(pkg, "Printf", luar.New(s.L, fmt.Printf))
	s.L.SetField(pkg, "Println", luar.New(s.L, fmt.Println))
	s.L.SetField(pkg, "Scan", luar.New(s.L, fmt.Scan))
	s.L.SetField(pkg, "Scanf", luar.New(s.L, fmt.Scanf))
	s.L.SetField(pkg, "Scanln", luar.New(s.L, fmt.Scanln))
	s.L.SetField(pkg, "Sprint", luar.New(s.L, fmt.Sprint))
	s.L.SetField(pkg, "Sprintf", luar.New(s.L, fmt.Sprintf))
	s.L.SetField(pkg, "Sprintln", luar.New(s.L, fmt.Sprintln))
	s.L.SetField(pkg, "Sscan", luar.New(s.L, fmt.Sscan))
	s.L.SetField(pkg, "Sscanf", luar.New(s.L, fmt.Sscanf))
	s.L.SetField(pkg, "Sscanln", luar.New(s.L, fmt.Sscanln))

	return pkg
}

func (s *State) importIo() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Copy", luar.New(s.L, io.Copy))
	s.L.SetField(pkg, "CopyN", luar.New(s.L, io.CopyN))
	s.L.SetField(pkg, "EOF", luar.New(s.L, io.EOF))
	s.L.SetField(pkg, "ErrClosedPipe", luar.New(s.L, io.ErrClosedPipe))
	s.L.SetField(pkg, "ErrNoProgress", luar.New(s.L, io.ErrNoProgress))
	s.L.SetField(pkg, "ErrShortBuffer", luar.New(s.L, io.ErrShortBuffer))
	s.L.SetField(pkg, "ErrShortWrite", luar.New(s.L, io.ErrShortWrite))
	s.L.SetField(pkg, "ErrUnexpectedEOF", luar.New(s.L, io.ErrUnexpectedEOF))
	s.L.SetField(pkg, "LimitReader", luar.New(s.L, io.LimitReader))
	s.L.SetField(pkg, "MultiReader", luar.New(s.L, io.MultiReader))
	s.L.SetField(pkg, "MultiWriter", luar.New(s.L, io.MultiWriter))
	s.L.SetField(pkg, "NewSectionReader", luar.New(s.L, io.NewSectionReader))
	s.L.SetField(pkg, "Pipe", luar.New(s.L, io.Pipe))
	s.L.SetField(pkg, "ReadAtLeast", luar.New(s.L, io.ReadAtLeast))
	s.L.SetField(pkg, "ReadFull", luar.New(s.L, io.ReadFull))
	s.L.SetField(pkg, "TeeReader", luar.New(s.L, io.TeeReader))
	s.L.SetField(pkg, "WriteString", luar.New(s.L, io.WriteString))

	return pkg
}

func (s *State) importIoUtil() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "ReadAll", luar.New(s.L, ioutil.ReadAll))
	s.L.SetField(pkg, "ReadDir", luar.New(s.L, ioutil.ReadDir))
	s.L.SetField(pkg, "ReadFile", luar.New(s.L, ioutil.ReadFile))
	s.L.SetField(pkg, "WriteFile", luar.New(s.L, ioutil.WriteFile))

	return pkg
}

func (s *State) importNet() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "CIDRMask", luar.New(s.L, net.CIDRMask))
	s.L.SetField(pkg, "Dial", luar.New(s.L, net.Dial))
	s.L.SetField(pkg, "DialIP", luar.New(s.L, net.DialIP))
	s.L.SetField(pkg, "DialTCP", luar.New(s.L, net.DialTCP))
	s.L.SetField(pkg, "DialTimeout", luar.New(s.L, net.DialTimeout))
	s.L.SetField(pkg, "DialUDP", luar.New(s.L, net.DialUDP))
	s.L.SetField(pkg, "DialUnix", luar.New(s.L, net.DialUnix))
	s.L.SetField(pkg, "ErrWriteToConnected", luar.New(s.L, net.ErrWriteToConnected))
	s.L.SetField(pkg, "FileConn", luar.New(s.L, net.FileConn))
	s.L.SetField(pkg, "FileListener", luar.New(s.L, net.FileListener))
	s.L.SetField(pkg, "FilePacketConn", luar.New(s.L, net.FilePacketConn))
	s.L.SetField(pkg, "FlagBroadcast", luar.New(s.L, net.FlagBroadcast))
	s.L.SetField(pkg, "FlagLoopback", luar.New(s.L, net.FlagLoopback))
	s.L.SetField(pkg, "FlagMulticast", luar.New(s.L, net.FlagMulticast))
	s.L.SetField(pkg, "FlagPointToPoint", luar.New(s.L, net.FlagPointToPoint))
	s.L.SetField(pkg, "FlagUp", luar.New(s.L, net.FlagUp))
	s.L.SetField(pkg, "IPv4", luar.New(s.L, net.IPv4))
	s.L.SetField(pkg, "IPv4Mask", luar.New(s.L, net.IPv4Mask))
	s.L.SetField(pkg, "IPv4allrouter", luar.New(s.L, net.IPv4allrouter))
	s.L.SetField(pkg, "IPv4allsys", luar.New(s.L, net.IPv4allsys))
	s.L.SetField(pkg, "IPv4bcast", luar.New(s.L, net.IPv4bcast))
	s.L.SetField(pkg, "IPv4len", luar.New(s.L, net.IPv4len))
	s.L.SetField(pkg, "IPv4zero", luar.New(s.L, net.IPv4zero))
	s.L.SetField(pkg, "IPv6interfacelocalallnodes", luar.New(s.L, net.IPv6interfacelocalallnodes))
	s.L.SetField(pkg, "IPv6len", luar.New(s.L, net.IPv6len))
	s.L.SetField(pkg, "IPv6linklocalallnodes", luar.New(s.L, net.IPv6linklocalallnodes))
	s.L.SetField(pkg, "IPv6linklocalallrouters", luar.New(s.L, net.IPv6linklocalallrouters))
	s.L.SetField(pkg, "IPv6loopback", luar.New(s.L, net.IPv6loopback))
	s.L.SetField(pkg, "IPv6unspecified", luar.New(s.L, net.IPv6unspecified))
	s.L.SetField(pkg, "IPv6zero", luar.New(s.L, net.IPv6zero))
	s.L.SetField(pkg, "InterfaceAddrs", luar.New(s.L, net.InterfaceAddrs))
	s.L.SetField(pkg, "InterfaceByIndex", luar.New(s.L, net.InterfaceByIndex))
	s.L.SetField(pkg, "InterfaceByName", luar.New(s.L, net.InterfaceByName))
	s.L.SetField(pkg, "Interfaces", luar.New(s.L, net.Interfaces))
	s.L.SetField(pkg, "JoinHostPort", luar.New(s.L, net.JoinHostPort))
	s.L.SetField(pkg, "Listen", luar.New(s.L, net.Listen))
	s.L.SetField(pkg, "ListenIP", luar.New(s.L, net.ListenIP))
	s.L.SetField(pkg, "ListenMulticastUDP", luar.New(s.L, net.ListenMulticastUDP))
	s.L.SetField(pkg, "ListenPacket", luar.New(s.L, net.ListenPacket))
	s.L.SetField(pkg, "ListenTCP", luar.New(s.L, net.ListenTCP))
	s.L.SetField(pkg, "ListenUDP", luar.New(s.L, net.ListenUDP))
	s.L.SetField(pkg, "ListenUnix", luar.New(s.L, net.ListenUnix))
	s.L.SetField(pkg, "ListenUnixgram", luar.New(s.L, net.ListenUnixgram))
	s.L.SetField(pkg, "LookupAddr", luar.New(s.L, net.LookupAddr))
	s.L.SetField(pkg, "LookupCNAME", luar.New(s.L, net.LookupCNAME))
	s.L.SetField(pkg, "LookupHost", luar.New(s.L, net.LookupHost))
	s.L.SetField(pkg, "LookupIP", luar.New(s.L, net.LookupIP))
	s.L.SetField(pkg, "LookupMX", luar.New(s.L, net.LookupMX))
	s.L.SetField(pkg, "LookupNS", luar.New(s.L, net.LookupNS))
	s.L.SetField(pkg, "LookupPort", luar.New(s.L, net.LookupPort))
	s.L.SetField(pkg, "LookupSRV", luar.New(s.L, net.LookupSRV))
	s.L.SetField(pkg, "LookupTXT", luar.New(s.L, net.LookupTXT))
	s.L.SetField(pkg, "ParseCIDR", luar.New(s.L, net.ParseCIDR))
	s.L.SetField(pkg, "ParseIP", luar.New(s.L, net.ParseIP))
	s.L.SetField(pkg, "ParseMAC", luar.New(s.L, net.ParseMAC))
	s.L.SetField(pkg, "Pipe", luar.New(s.L, net.Pipe))
	s.L.SetField(pkg, "ResolveIPAddr", luar.New(s.L, net.ResolveIPAddr))
	s.L.SetField(pkg, "ResolveTCPAddr", luar.New(s.L, net.ResolveTCPAddr))
	s.L.SetField(pkg, "ResolveUDPAddr", luar.New(s.L, net.ResolveUDPAddr))
	s.L.SetField(pkg, "ResolveUnixAddr", luar.New(s.L, net.ResolveUnixAddr))
	s.L.SetField(pkg, "SplitHostPort", luar.New(s.L, net.SplitHostPort))

	return pkg
}

func (s *State) importMath() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Abs", luar.New(s.L, math.Abs))
	s.L.SetField(pkg, "Acos", luar.New(s.L, math.Acos))
	s.L.SetField(pkg, "Acosh", luar.New(s.L, math.Acosh))
	s.L.SetField(pkg, "Asin", luar.New(s.L, math.Asin))
	s.L.SetField(pkg, "Asinh", luar.New(s.L, math.Asinh))
	s.L.SetField(pkg, "Atan", luar.New(s.L, math.Atan))
	s.L.SetField(pkg, "Atan2", luar.New(s.L, math.Atan2))
	s.L.SetField(pkg, "Atanh", luar.New(s.L, math.Atanh))
	s.L.SetField(pkg, "Cbrt", luar.New(s.L, math.Cbrt))
	s.L.SetField(pkg, "Ceil", luar.New(s.L, math.Ceil))
	s.L.SetField(pkg, "Copysign", luar.New(s.L, math.Copysign))
	s.L.SetField(pkg, "Cos", luar.New(s.L, math.Cos))
	s.L.SetField(pkg, "Cosh", luar.New(s.L, math.Cosh))
	s.L.SetField(pkg, "Dim", luar.New(s.L, math.Dim))
	s.L.SetField(pkg, "Erf", luar.New(s.L, math.Erf))
	s.L.SetField(pkg, "Erfc", luar.New(s.L, math.Erfc))
	s.L.SetField(pkg, "Exp", luar.New(s.L, math.Exp))
	s.L.SetField(pkg, "Exp2", luar.New(s.L, math.Exp2))
	s.L.SetField(pkg, "Expm1", luar.New(s.L, math.Expm1))
	s.L.SetField(pkg, "Float32bits", luar.New(s.L, math.Float32bits))
	s.L.SetField(pkg, "Float32frombits", luar.New(s.L, math.Float32frombits))
	s.L.SetField(pkg, "Float64bits", luar.New(s.L, math.Float64bits))
	s.L.SetField(pkg, "Float64frombits", luar.New(s.L, math.Float64frombits))
	s.L.SetField(pkg, "Floor", luar.New(s.L, math.Floor))
	s.L.SetField(pkg, "Frexp", luar.New(s.L, math.Frexp))
	s.L.SetField(pkg, "Gamma", luar.New(s.L, math.Gamma))
	s.L.SetField(pkg, "Hypot", luar.New(s.L, math.Hypot))
	s.L.SetField(pkg, "Ilogb", luar.New(s.L, math.Ilogb))
	s.L.SetField(pkg, "Inf", luar.New(s.L, math.Inf))
	s.L.SetField(pkg, "IsInf", luar.New(s.L, math.IsInf))
	s.L.SetField(pkg, "IsNaN", luar.New(s.L, math.IsNaN))
	s.L.SetField(pkg, "J0", luar.New(s.L, math.J0))
	s.L.SetField(pkg, "J1", luar.New(s.L, math.J1))
	s.L.SetField(pkg, "Jn", luar.New(s.L, math.Jn))
	s.L.SetField(pkg, "Ldexp", luar.New(s.L, math.Ldexp))
	s.L.SetField(pkg, "Lgamma", luar.New(s.L, math.Lgamma))
	s.L.SetField(pkg, "Log", luar.New(s.L, math.Log))
	s.L.SetField(pkg, "Log10", luar.New(s.L, math.Log10))
	s.L.SetField(pkg, "Log1p", luar.New(s.L, math.Log1p))
	s.L.SetField(pkg, "Log2", luar.New(s.L, math.Log2))
	s.L.SetField(pkg, "Logb", luar.New(s.L, math.Logb))
	s.L.SetField(pkg, "Max", luar.New(s.L, math.Max))
	s.L.SetField(pkg, "Min", luar.New(s.L, math.Min))
	s.L.SetField(pkg, "Mod", luar.New(s.L, math.Mod))
	s.L.SetField(pkg, "Modf", luar.New(s.L, math.Modf))
	s.L.SetField(pkg, "NaN", luar.New(s.L, math.NaN))
	s.L.SetField(pkg, "Nextafter", luar.New(s.L, math.Nextafter))
	s.L.SetField(pkg, "Pow", luar.New(s.L, math.Pow))
	s.L.SetField(pkg, "Pow10", luar.New(s.L, math.Pow10))
	s.L.SetField(pkg, "Remainder", luar.New(s.L, math.Remainder))
	s.L.SetField(pkg, "Signbit", luar.New(s.L, math.Signbit))
	s.L.SetField(pkg, "Sin", luar.New(s.L, math.Sin))
	s.L.SetField(pkg, "Sincos", luar.New(s.L, math.Sincos))
	s.L.SetField(pkg, "Sinh", luar.New(s.L, math.Sinh))
	s.L.SetField(pkg, "Sqrt", luar.New(s.L, math.Sqrt))
	s.L.SetField(pkg, "Tan", luar.New(s.L, math.Tan))
	s.L.SetField(pkg, "Tanh", luar.New(s.L, math.Tanh))
	s.L.SetField(pkg, "Trunc", luar.New(s.L, math.Trunc))
	s.L.SetField(pkg, "Y0", luar.New(s.L, math.Y0))
	s.L.SetField(pkg, "Y1", luar.New(s.L, math.Y1))
	s.L.SetField(pkg, "Yn", luar.New(s.L, math.Yn))

	return pkg
}

func (s *State) importMathRand() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "ExpFloat64", luar.New(s.L, rand.ExpFloat64))
	s.L.SetField(pkg, "Float32", luar.New(s.L, rand.Float32))
	s.L.SetField(pkg, "Float64", luar.New(s.L, rand.Float64))
	s.L.SetField(pkg, "Int", luar.New(s.L, rand.Int))
	s.L.SetField(pkg, "Int31", luar.New(s.L, rand.Int31))
	s.L.SetField(pkg, "Int31n", luar.New(s.L, rand.Int31n))
	s.L.SetField(pkg, "Int63", luar.New(s.L, rand.Int63))
	s.L.SetField(pkg, "Int63n", luar.New(s.L, rand.Int63n))
	s.L.SetField(pkg, "Intn", luar.New(s.L, rand.Intn))
	s.L.SetField(pkg, "NormFloat64", luar.New(s.L, rand.NormFloat64))
	s.L.SetField(pkg, "Perm", luar.New(s.L, rand.Perm))
	s.L.SetField(pkg, "Seed", luar.New(s.L, rand.Seed))
	s.L.SetField(pkg, "Uint32", luar.New(s.L, rand.Uint32))

	return pkg
}

func (s *State) importOs() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Args", luar.New(s.L, os.Args))
	s.L.SetField(pkg, "Chdir", luar.New(s.L, os.Chdir))
	s.L.SetField(pkg, "Chmod", luar.New(s.L, os.Chmod))
	s.L.SetField(pkg, "Chown", luar.New(s.L, os.Chown))
	s.L.SetField(pkg, "Chtimes", luar.New(s.L, os.Chtimes))
	s.L.SetField(pkg, "Clearenv", luar.New(s.L, os.Clearenv))
	s.L.SetField(pkg, "Create", luar.New(s.L, os.Create))
	s.L.SetField(pkg, "DevNull", luar.New(s.L, os.DevNull))
	s.L.SetField(pkg, "Environ", luar.New(s.L, os.Environ))
	s.L.SetField(pkg, "ErrExist", luar.New(s.L, os.ErrExist))
	s.L.SetField(pkg, "ErrInvalid", luar.New(s.L, os.ErrInvalid))
	s.L.SetField(pkg, "ErrNotExist", luar.New(s.L, os.ErrNotExist))
	s.L.SetField(pkg, "ErrPermission", luar.New(s.L, os.ErrPermission))
	s.L.SetField(pkg, "Exit", luar.New(s.L, os.Exit))
	s.L.SetField(pkg, "Expand", luar.New(s.L, os.Expand))
	s.L.SetField(pkg, "ExpandEnv", luar.New(s.L, os.ExpandEnv))
	s.L.SetField(pkg, "FindProcess", luar.New(s.L, os.FindProcess))
	s.L.SetField(pkg, "Getegid", luar.New(s.L, os.Getegid))
	s.L.SetField(pkg, "Getenv", luar.New(s.L, os.Getenv))
	s.L.SetField(pkg, "Geteuid", luar.New(s.L, os.Geteuid))
	s.L.SetField(pkg, "Getgid", luar.New(s.L, os.Getgid))
	s.L.SetField(pkg, "Getgroups", luar.New(s.L, os.Getgroups))
	s.L.SetField(pkg, "Getpagesize", luar.New(s.L, os.Getpagesize))
	s.L.SetField(pkg, "Getpid", luar.New(s.L, os.Getpid))
	s.L.SetField(pkg, "Getuid", luar.New(s.L, os.Getuid))
	s.L.SetField(pkg, "Getwd", luar.New(s.L, os.Getwd))
	s.L.SetField(pkg, "Hostname", luar.New(s.L, os.Hostname))
	s.L.SetField(pkg, "Interrupt", luar.New(s.L, os.Interrupt))
	s.L.SetField(pkg, "IsExist", luar.New(s.L, os.IsExist))
	s.L.SetField(pkg, "IsNotExist", luar.New(s.L, os.IsNotExist))
	s.L.SetField(pkg, "IsPathSeparator", luar.New(s.L, os.IsPathSeparator))
	s.L.SetField(pkg, "IsPermission", luar.New(s.L, os.IsPermission))
	s.L.SetField(pkg, "Kill", luar.New(s.L, os.Kill))
	s.L.SetField(pkg, "Lchown", luar.New(s.L, os.Lchown))
	s.L.SetField(pkg, "Link", luar.New(s.L, os.Link))
	s.L.SetField(pkg, "Lstat", luar.New(s.L, os.Lstat))
	s.L.SetField(pkg, "Mkdir", luar.New(s.L, os.Mkdir))
	s.L.SetField(pkg, "MkdirAll", luar.New(s.L, os.MkdirAll))
	s.L.SetField(pkg, "ModeAppend", luar.New(s.L, os.ModeAppend))
	s.L.SetField(pkg, "ModeCharDevice", luar.New(s.L, os.ModeCharDevice))
	s.L.SetField(pkg, "ModeDevice", luar.New(s.L, os.ModeDevice))
	s.L.SetField(pkg, "ModeDir", luar.New(s.L, os.ModeDir))
	s.L.SetField(pkg, "ModeExclusive", luar.New(s.L, os.ModeExclusive))
	s.L.SetField(pkg, "ModeNamedPipe", luar.New(s.L, os.ModeNamedPipe))
	s.L.SetField(pkg, "ModePerm", luar.New(s.L, os.ModePerm))
	s.L.SetField(pkg, "ModeSetgid", luar.New(s.L, os.ModeSetgid))
	s.L.SetField(pkg, "ModeSetuid", luar.New(s.L, os.ModeSetuid))
	s.L.SetField(pkg, "ModeSocket", luar.New(s.L, os.ModeSocket))
	s.L.SetField(pkg, "ModeSticky", luar.New(s.L, os.ModeSticky))
	s.L.SetField(pkg, "ModeSymlink", luar.New(s.L, os.ModeSymlink))
	s.L.SetField(pkg, "ModeTemporary", luar.New(s.L, os.ModeTemporary))
	s.L.SetField(pkg, "ModeType", luar.New(s.L, os.ModeType))
	s.L.SetField(pkg, "NewFile", luar.New(s.L, os.NewFile))
	s.L.SetField(pkg, "NewSyscallError", luar.New(s.L, os.NewSyscallError))
	s.L.SetField(pkg, "O_APPEND", luar.New(s.L, os.O_APPEND))
	s.L.SetField(pkg, "O_CREATE", luar.New(s.L, os.O_CREATE))
	s.L.SetField(pkg, "O_EXCL", luar.New(s.L, os.O_EXCL))
	s.L.SetField(pkg, "O_RDONLY", luar.New(s.L, os.O_RDONLY))
	s.L.SetField(pkg, "O_RDWR", luar.New(s.L, os.O_RDWR))
	s.L.SetField(pkg, "O_SYNC", luar.New(s.L, os.O_SYNC))
	s.L.SetField(pkg, "O_TRUNC", luar.New(s.L, os.O_TRUNC))
	s.L.SetField(pkg, "O_WRONLY", luar.New(s.L, os.O_WRONLY))
	s.L.SetField(pkg, "Open", luar.New(s.L, os.Open))
	s.L.SetField(pkg, "OpenFile", luar.New(s.L, os.OpenFile))
	s.L.SetField(pkg, "PathListSeparator", luar.New(s.L, os.PathListSeparator))
	s.L.SetField(pkg, "PathSeparator", luar.New(s.L, os.PathSeparator))
	s.L.SetField(pkg, "Pipe", luar.New(s.L, os.Pipe))
	s.L.SetField(pkg, "Readlink", luar.New(s.L, os.Readlink))
	s.L.SetField(pkg, "Remove", luar.New(s.L, os.Remove))
	s.L.SetField(pkg, "RemoveAll", luar.New(s.L, os.RemoveAll))
	s.L.SetField(pkg, "Rename", luar.New(s.L, os.Rename))
	s.L.SetField(pkg, "SEEK_CUR", luar.New(s.L, os.SEEK_CUR))
	s.L.SetField(pkg, "SEEK_END", luar.New(s.L, os.SEEK_END))
	s.L.SetField(pkg, "SEEK_SET", luar.New(s.L, os.SEEK_SET))
	s.L.SetField(pkg, "SameFile", luar.New(s.L, os.SameFile))
	s.L.SetField(pkg, "Setenv", luar.New(s.L, os.Setenv))
	s.L.SetField(pkg, "StartProcess", luar.New(s.L, os.StartProcess))
	s.L.SetField(pkg, "Stat", luar.New(s.L, os.Stat))
	s.L.SetField(pkg, "Stderr", luar.New(s.L, os.Stderr))
	s.L.SetField(pkg, "Stdin", luar.New(s.L, os.Stdin))
	s.L.SetField(pkg, "Stdout", luar.New(s.L, os.Stdout))
	s.L.SetField(pkg, "Symlink", luar.New(s.L, os.Symlink))
	s.L.SetField(pkg, "TempDir", luar.New(s.L, os.TempDir))
	s.L.SetField(pkg, "Truncate", luar.New(s.L, os.Truncate))

	return pkg
}

func (s *State) importRuntime() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "GC", luar.New(s.L, runtime.GC))
	s.L.SetField(pkg, "GOARCH", luar.New(s.L, runtime.GOARCH))
	s.L.SetField(pkg, "GOMAXPROCS", luar.New(s.L, runtime.GOMAXPROCS))
	s.L.SetField(pkg, "GOOS", luar.New(s.L, runtime.GOOS))
	s.L.SetField(pkg, "GOROOT", luar.New(s.L, runtime.GOROOT))

	return pkg
}

func (s *State) importPath() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Base", luar.New(s.L, path.Base))
	s.L.SetField(pkg, "Clean", luar.New(s.L, path.Clean))
	s.L.SetField(pkg, "Dir", luar.New(s.L, path.Dir))
	s.L.SetField(pkg, "ErrBadPattern", luar.New(s.L, path.ErrBadPattern))
	s.L.SetField(pkg, "Ext", luar.New(s.L, path.Ext))
	s.L.SetField(pkg, "IsAbs", luar.New(s.L, path.IsAbs))
	s.L.SetField(pkg, "Join", luar.New(s.L, path.Join))
	s.L.SetField(pkg, "Match", luar.New(s.L, path.Match))
	s.L.SetField(pkg, "Split", luar.New(s.L, path.Split))

	return pkg
}

func (s *State) importFilePath() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Join", luar.New(s.L, filepath.Join))
	s.L.SetField(pkg, "Abs", luar.New(s.L, filepath.Abs))
	s.L.SetField(pkg, "Base", luar.New(s.L, filepath.Base))
	s.L.SetField(pkg, "Clean", luar.New(s.L, filepath.Clean))
	s.L.SetField(pkg, "Dir", luar.New(s.L, filepath.Dir))
	s.L.SetField(pkg, "EvalSymlinks", luar.New(s.L, filepath.EvalSymlinks))
	s.L.SetField(pkg, "Ext", luar.New(s.L, filepath.Ext))
	s.L.SetField(pkg, "FromSlash", luar.New(s.L, filepath.FromSlash))
	s.L.SetField(pkg, "Glob", luar.New(s.L, filepath.Glob))
	s.L.SetField(pkg, "HasPrefix", luar.New(s.L, filepath.HasPrefix))
	s.L.SetField(pkg, "IsAbs", luar.New(s.L, filepath.IsAbs))
	s.L.SetField(pkg, "Join", luar.New(s.L, filepath.Join))
	s.L.SetField(pkg, "Match", luar.New(s.L, filepath.Match))
	s.L.SetField(pkg, "Rel", luar.New(s.L, filepath.Rel))
	s.L.SetField(pkg, "Split", luar.New(s.L, filepath.Split))
	s.L.SetField(pkg, "SplitList", luar.New(s.L, filepath.SplitList))
	s.L.SetField(pkg, "ToSlash", luar.New(s.L, filepath.ToSlash))
	s.L.SetField(pkg, "VolumeName", luar.New(s.L, filepath.VolumeName))

	return pkg
}

func (s *State) importStrings() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Contains", luar.New(s.L, strings.Contains))
	s.L.SetField(pkg, "ContainsAny", luar.New(s.L, strings.ContainsAny))
	s.L.SetField(pkg, "ContainsRune", luar.New(s.L, strings.ContainsRune))
	s.L.SetField(pkg, "Count", luar.New(s.L, strings.Count))
	s.L.SetField(pkg, "EqualFold", luar.New(s.L, strings.EqualFold))
	s.L.SetField(pkg, "Fields", luar.New(s.L, strings.Fields))
	s.L.SetField(pkg, "FieldsFunc", luar.New(s.L, strings.FieldsFunc))
	s.L.SetField(pkg, "HasPrefix", luar.New(s.L, strings.HasPrefix))
	s.L.SetField(pkg, "HasSuffix", luar.New(s.L, strings.HasSuffix))
	s.L.SetField(pkg, "Index", luar.New(s.L, strings.Index))
	s.L.SetField(pkg, "IndexAny", luar.New(s.L, strings.IndexAny))
	s.L.SetField(pkg, "IndexByte", luar.New(s.L, strings.IndexByte))
	s.L.SetField(pkg, "IndexFunc", luar.New(s.L, strings.IndexFunc))
	s.L.SetField(pkg, "IndexRune", luar.New(s.L, strings.IndexRune))
	s.L.SetField(pkg, "Join", luar.New(s.L, strings.Join))
	s.L.SetField(pkg, "LastIndex", luar.New(s.L, strings.LastIndex))
	s.L.SetField(pkg, "LastIndexAny", luar.New(s.L, strings.LastIndexAny))
	s.L.SetField(pkg, "LastIndexFunc", luar.New(s.L, strings.LastIndexFunc))
	s.L.SetField(pkg, "Map", luar.New(s.L, strings.Map))
	s.L.SetField(pkg, "NewReader", luar.New(s.L, strings.NewReader))
	s.L.SetField(pkg, "NewReplacer", luar.New(s.L, strings.NewReplacer))
	s.L.SetField(pkg, "Repeat", luar.New(s.L, strings.Repeat))
	s.L.SetField(pkg, "Replace", luar.New(s.L, strings.Replace))
	s.L.SetField(pkg, "Split", luar.New(s.L, strings.Split))
	s.L.SetField(pkg, "SplitAfter", luar.New(s.L, strings.SplitAfter))
	s.L.SetField(pkg, "SplitAfterN", luar.New(s.L, strings.SplitAfterN))
	s.L.SetField(pkg, "SplitN", luar.New(s.L, strings.SplitN))
	s.L.SetField(pkg, "Title", luar.New(s.L, strings.Title))
	s.L.SetField(pkg, "ToLower", luar.New(s.L, strings.ToLower))
	s.L.SetField(pkg, "ToLowerSpecial", luar.New(s.L, strings.ToLowerSpecial))
	s.L.SetField(pkg, "ToTitle", luar.New(s.L, strings.ToTitle))
	s.L.SetField(pkg, "ToTitleSpecial", luar.New(s.L, strings.ToTitleSpecial))
	s.L.SetField(pkg, "ToUpper", luar.New(s.L, strings.ToUpper))
	s.L.SetField(pkg, "ToUpperSpecial", luar.New(s.L, strings.ToUpperSpecial))
	s.L.SetField(pkg, "Trim", luar.New(s.L, strings.Trim))
	s.L.SetField(pkg, "TrimFunc", luar.New(s.L, strings.TrimFunc))
	s.L.SetField(pkg, "TrimLeft", luar.New(s.L, strings.TrimLeft))
	s.L.SetField(pkg, "TrimLeftFunc", luar.New(s.L, strings.TrimLeftFunc))
	s.L.SetField(pkg, "TrimPrefix", luar.New(s.L, strings.TrimPrefix))
	s.L.SetField(pkg, "TrimRight", luar.New(s.L, strings.TrimRight))
	s.L.SetField(pkg, "TrimRightFunc", luar.New(s.L, strings.TrimRightFunc))
	s.L.SetField(pkg, "TrimSpace", luar.New(s.L, strings.TrimSpace))
	s.L.SetField(pkg, "TrimSuffix", luar.New(s.L, strings.TrimSuffix))

	return pkg
}

func (s *State) importRegexp() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Match", luar.New(s.L, regexp.Match))
	s.L.SetField(pkg, "MatchReader", luar.New(s.L, regexp.MatchReader))
	s.L.SetField(pkg, "MatchString", luar.New(s.L, regexp.MatchString))
	s.L.SetField(pkg, "QuoteMeta", luar.New(s.L, regexp.QuoteMeta))
	s.L.SetField(pkg, "Compile", luar.New(s.L, regexp.Compile))
	s.L.SetField(pkg, "CompilePOSIX", luar.New(s.L, regexp.CompilePOSIX))
	s.L.SetField(pkg, "MustCompile", luar.New(s.L, regexp.MustCompile))
	s.L.SetField(pkg, "MustCompilePOSIX", luar.New(s.L, regexp.MustCompilePOSIX))

	return pkg
}

func (s *State) importErrors() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "New", luar.New(s.L, errors.New))

	return pkg
}

func (s *State) importTime() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "After", luar.New(s.L, time.After))
	s.L.SetField(pkg, "Sleep", luar.New(s.L, time.Sleep))
	s.L.SetField(pkg, "Tick", luar.New(s.L, time.Tick))
	s.L.SetField(pkg, "Since", luar.New(s.L, time.Since))
	s.L.SetField(pkg, "FixedZone", luar.New(s.L, time.FixedZone))
	s.L.SetField(pkg, "LoadLocation", luar.New(s.L, time.LoadLocation))
	s.L.SetField(pkg, "NewTicker", luar.New(s.L, time.NewTicker))
	s.L.SetField(pkg, "Date", luar.New(s.L, time.Date))
	s.L.SetField(pkg, "Now", luar.New(s.L, time.Now))
	s.L.SetField(pkg, "Parse", luar.New(s.L, time.Parse))
	s.L.SetField(pkg, "ParseDuration", luar.New(s.L, time.ParseDuration))
	s.L.SetField(pkg, "ParseInLocation", luar.New(s.L, time.ParseInLocation))
	s.L.SetField(pkg, "Unix", luar.New(s.L, time.Unix))
	s.L.SetField(pkg, "AfterFunc", luar.New(s.L, time.AfterFunc))
	s.L.SetField(pkg, "NewTimer", luar.New(s.L, time.NewTimer))
	s.L.SetField(pkg, "Nanosecond", luar.New(s.L, time.Nanosecond))
	s.L.SetField(pkg, "Microsecond", luar.New(s.L, time.Microsecond))
	s.L.SetField(pkg, "Millisecond", luar.New(s.L, time.Millisecond))
	s.L.SetField(pkg, "Second", luar.New(s.L, time.Second))
	s.L.SetField(pkg, "Minute", luar.New(s.L, time.Minute))
	s.L.SetField(pkg, "Hour", luar.New(s.L, time.Hour))

	return pkg
}

func (s *State) importUtf8() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "DecodeLastRune", luar.New(s.L, utf8.DecodeLastRune))
	s.L.SetField(pkg, "DecodeLastRuneInString", luar.New(s.L, utf8.DecodeLastRuneInString))
	s.L.SetField(pkg, "DecodeRune", luar.New(s.L, utf8.DecodeRune))
	s.L.SetField(pkg, "DecodeRuneInString", luar.New(s.L, utf8.DecodeRuneInString))
	s.L.SetField(pkg, "EncodeRune", luar.New(s.L, utf8.EncodeRune))
	s.L.SetField(pkg, "FullRune", luar.New(s.L, utf8.FullRune))
	s.L.SetField(pkg, "FullRuneInString", luar.New(s.L, utf8.FullRuneInString))
	s.L.SetField(pkg, "RuneCount", luar.New(s.L, utf8.RuneCount))
	s.L.SetField(pkg, "RuneCountInString", luar.New(s.L, utf8.RuneCountInString))
	s.L.SetField(pkg, "RuneLen", luar.New(s.L, utf8.RuneLen))
	s.L.SetField(pkg, "RuneStart", luar.New(s.L, utf8.RuneStart))
	s.L.SetField(pkg, "Valid", luar.New(s.L, utf8.Valid))
	s.L.SetField(pkg, "ValidRune", luar.New(s.L, utf8.ValidRune))
	s.L.SetField(pkg, "ValidString", luar.New(s.L, utf8.ValidString))

	return pkg
}

func (s *State) importHumanize() *lua.LTable {
	pkg := s.L.NewTable()

	s.L.SetField(pkg, "Bytes", luar.New(s.L, humanize.Bytes))
	s.L.SetField(pkg, "Ordinal", luar.New(s.L, humanize.Ordinal))

	return pkg
}
