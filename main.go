package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/prometheus/procfs"
	"github.com/samber/lo"
	"github.com/sanity-io/litter"
)

var _ = litter.Dump

// ignoreExecutableList is the set of processes to ignore when traversing to find the deepest
// children of a given process. Done because gopls is often the deepest child and will have a CWD of
// `~/.config/go/telemetry/local`, and in that case we want to just ignore gopls.
var ignoreExecutableList []string = []string{
	"gopls",
	"",
	".",
}

func main() {
	cwd, err := getCurrentWorkingDirectory()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Println(cwd)
}

func getCurrentWorkingDirectory() (string, error) {
	pid, err := getWindowPID()
	if err != nil {
		return "", fmt.Errorf("failed to get window PID: %w", err)
	}
	dbgprint(os.Stderr, "# PID: %d\n", pid)

	// Now get all the processes and organize them by their pid.
	procs, err := procfs.AllProcs()
	if err != nil {
		return "", fmt.Errorf("failed to get all processes: %w", err)
	}

	procsByPPID := lo.GroupBy(procs, func(p procfs.Proc) int { return must(p.Stat()).PPID })
	procsByPID := lo.GroupBy(procs, func(p procfs.Proc) int { return p.PID })

	// search recursively for processes which are children of
	deepestChildPID, depth := findDeepestChild(procsByPPID, procsByPID, int(pid), 0)
	var _ = depth
	var _ = deepestChildPID
	dbgprint(os.Stderr, "# Depth: %d  PID: %d\n", depth, deepestChildPID)

	return procsByPID[deepestChildPID][0].Cwd()
}

func findDeepestChild(procsByPPID, procsByPID map[int]procfs.Procs, pid, depth int) (int, int64) {
	indent := strings.Repeat("    ", depth)
	p := procsByPID[pid][0]
	dbgprint(os.Stderr, "# %s%d %v %s %q %q\n",
		indent,
		pid,
		isProcAllowed(p),
		must(p.Executable()),
		strings.Join(must(p.CmdLine()), ` `),
		must(p.Cwd()))
	children := procsByPPID[pid]
	if len(children) == 0 {
		return pid, 0
	}
	longestDescDepth := int64(0)
	longestDescPID := pid
	for _, child := range children {
		descPID, descDepth := findDeepestChild(procsByPPID, procsByPID, child.PID, depth+1)
		dp := procsByPID[descPID][0]
		if descDepth >= longestDescDepth && isProcAllowed(dp) {
			longestDescDepth = descDepth
			longestDescPID = descPID
		}
	}
	return longestDescPID, longestDescDepth + 1
}

func isProcAllowed(p procfs.Proc) bool {
	baseExeName := path.Base(must(p.Executable()))
	return !slices.Contains(ignoreExecutableList, baseExeName)
}

func getWindowPID() (uint32, error) {
	// Connect to X server
	conn, err := xgb.NewConn()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to X server: %w", err)
	}
	defer conn.Close()

	// Get the setup info
	setup := xproto.Setup(conn)
	root := setup.DefaultScreen(conn).Root

	// Get the currently focused window
	focusReply, err := xproto.GetInputFocus(conn).Reply()
	if err != nil {
		return 0, fmt.Errorf("failed to get focused window: %w", err)
	}

	focusedWindow := focusReply.Focus

	// If the focused window is the root or none, we can't get a PID
	if focusedWindow == xproto.WindowNone || focusedWindow == root {
		return 0, fmt.Errorf("no window is focused")
	}

	// Get the atom for _NET_WM_PID
	atomReply, err := xproto.InternAtom(conn, false, uint16(len("_NET_WM_PID")), "_NET_WM_PID").Reply()
	if err != nil {
		return 0, fmt.Errorf("failed to get _NET_WM_PID atom: %w", err)
	}

	// Get the property value
	propReply, err := xproto.GetProperty(conn, false, focusedWindow, atomReply.Atom, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil {
		return 0, fmt.Errorf("failed to get property: %w", err)
	}

	if propReply.ValueLen == 0 {
		return 0, fmt.Errorf("_NET_WM_PID property not set")
	}

	// The PID is stored as a 32-bit cardinal
	pid := uint32(propReply.Value[0]) |
		uint32(propReply.Value[1])<<8 |
		uint32(propReply.Value[2])<<16 |
		uint32(propReply.Value[3])<<24

	return pid, nil
}

func dbgprint(w io.Writer, format string, a ...any) (n int, err error) {
	if os.Getenv("DEBUG") != "" {
		return fmt.Fprintf(w, format, a...)
	}
	return 0, nil
}

func must[T any](x T, err error) T {
	if err != nil {
		panic(err)
	}
	return x
}
