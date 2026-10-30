// Command gguard-setup installs, upgrades and removes GopherGuard.
//
// It owns the installation so the main binary does not have to: it carries the
// binaries it writes (gguard and, on Windows, gguard-cli), registers the
// service, the shortcuts and the tray setting, and keeps a copy of itself in
// the install directory so upgrade and uninstall always stay available.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/KaioHSG/gguard/install"
)

var (
	userFlag    bool
	scriptsFlag scriptsList
	guardsFlag  string
)

type scriptsList []string

func (s *scriptsList) String() string { return strings.Join(*s, ", ") }
func (s *scriptsList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func registerFlags(fs *flag.FlagSet) {
	fs.BoolVar(&userFlag, "user", false, "Install per-user instead of system-wide")
	fs.Var(&scriptsFlag, "scripts", ".ggs file or directory to install")
	fs.Var(&scriptsFlag, "s", "")
	fs.Var(&scriptsFlag, "script", "")
	fs.StringVar(&guardsFlag, "guards", "", "guards.json to install")
	fs.StringVar(&guardsFlag, "g", "", "")
}

func init() {
	registerFlags(flag.CommandLine)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `gguard-setup - installer for GopherGuard v%s

Usage:
  gguard-setup                      Show the installer dialog
  gguard-setup <command> [flags]

Commands:
  install [--user]     Install GopherGuard (--user for a per-user install)
  upgrade              Reinstall, keeping the same install type
  uninstall [--user]   Remove GopherGuard and its service
  version              Show version
  help                 Show this help

Flags:
  --user               Per-user install (no administrator rights needed)
  -s, --scripts <path> .ggs file or directory to install
  -g, --guards <path>  guards.json to install
`, install.Version)
	}
}

func main() {
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--") && len(arg) > 2 && arg[2] != '-' {
			os.Args[i] = "-" + arg[2:]
		}
	}

	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		showDialog()
		return
	}

	sub := flag.NewFlagSet(args[0], flag.ExitOnError)
	registerFlags(sub)
	sub.Usage = flag.Usage
	_ = sub.Parse(args[1:])

	switch args[0] {
	case "install":
		doInstall(userFlag)
	case "uninstall":
		doUninstall(userFlag)
	case "upgrade":
		doUpgrade()
	case "version":
		fmt.Printf("gguard-setup v%s\n", install.Version)
	case "help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "gguard-setup: unknown command: %s\n", args[0])
		flag.Usage()
		os.Exit(1)
	}
}

// showDialog is what a double-clicked installer shows.
func showDialog() {
	userService, installed := install.DetectType()

	fmt.Println("=== GopherGuard Installer ===")
	fmt.Printf("    version %s\n", install.Version)
	fmt.Println()

	choice, ok := prompt(installed, userService, install.IsAdmin())
	if !ok {
		return
	}

	switch choice {
	case "a":
		if !install.IsAdmin() {
			fmt.Println("Requesting administrator privileges...")
			startElevated([]string{"install"})
			return
		}
		doInstall(false)
	case "u":
		doInstall(true)
	case "r":
		doUninstall(userService)
	case "g":
		doUpgrade()
	}

	pauseBeforeExit()
}

// prompt prints the menu and returns the chosen action, or ok=false when the
// user closed the window.
func prompt(installed, userService, admin bool) (action string, ok bool) {
	fmt.Println()

	if installed {
		kind := "system"
		if userService {
			kind = "user"
		}
		fmt.Printf("GopherGuard is already installed (%s).\n", kind)
		fmt.Println("[U] Upgrade")
		fmt.Println("[R] Uninstall")
		fmt.Println("[C] Cancel")
	} else {
		fmt.Println("GopherGuard is not installed.")
		if admin {
			fmt.Println("[A] System install (all users)")
		} else {
			fmt.Println("[A] System install (needs administrator rights)")
		}
		fmt.Println("[U] Per-user install")
		fmt.Println("[C] Cancel")
	}

	fmt.Print("Choice: ")
	line, _, err := bufio.NewReader(os.Stdin).ReadLine()
	if err == io.EOF || err != nil {
		return "", false
	}

	switch strings.ToLower(strings.TrimSpace(string(line))) {
	case "a":
		return "a", true
	case "u":
		if installed {
			return "g", true
		}
		return "u", true
	case "r":
		return "r", true
	}
	return "", true
}

func pauseBeforeExit() {
	fmt.Println()
	fmt.Print("Press Enter to close...")
	_, _, _ = bufio.NewReader(os.Stdin).ReadLine()
}

// warn logs a non-fatal problem so the install can still finish.
func warn(format string, args ...any) {
	log.Printf("warning: "+format, args...)
}
