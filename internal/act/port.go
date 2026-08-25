package act

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	PortMin = 8180
	PortMax = 8399
)

// NextPort picks the first port in 8180-8399 that is not used, is not the
// template port, and passes bindOK. bindOK nil means every candidate binds.
func NextPort(used []int, template int, bindOK func(int) bool) (int, error) {
	skip := map[int]struct{}{}
	if template != 0 {
		skip[template] = struct{}{}
	}
	for _, p := range used {
		skip[p] = struct{}{}
	}
	if bindOK == nil {
		bindOK = func(int) bool { return true }
	}
	for p := PortMin; p <= PortMax; p++ {
		if _, hit := skip[p]; hit {
			continue
		}
		if !bindOK(p) {
			continue
		}
		return p, nil
	}
	return 0, fmt.Errorf("no free port in %d-%d", PortMin, PortMax)
}

// PortFromArgv reads --port N or --port=N. Missing is 0.
func PortFromArgv(argv []string) int {
	for i, a := range argv {
		switch {
		case a == "--port" && i+1 < len(argv):
			p, _ := strconv.Atoi(argv[i+1])
			return p
		case strings.HasPrefix(a, "--port="):
			p, _ := strconv.Atoi(strings.TrimPrefix(a, "--port="))
			return p
		}
	}
	return 0
}

// BindOK is the production bind check: listen on loopback and close.
func BindOK(port int) bool {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
