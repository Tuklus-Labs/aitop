package act

import "testing"

func TestNextPortSkipsUsedAndTemplate(t *testing.T) {
	p, err := NextPort([]int{8193, 8180}, 8193, func(int) bool { return true })
	if err != nil {
		t.Fatalf("next-port: %v", err)
	}
	if p == 8193 {
		t.Fatalf("next-port-skips-used-and-template violated: got template port 8193")
	}
	if p == 8180 {
		t.Fatalf("next-port-skips-used-and-template violated: got used port 8180")
	}
	if p < PortMin || p > PortMax {
		t.Fatalf("next-port-skips-used-and-template violated: %d not in %d-%d", p, PortMin, PortMax)
	}
	if p != 8181 {
		t.Fatalf("next-port-skips-used-and-template violated: first free want 8181 got %d", p)
	}
}

func TestNextPortSkipsBindFail(t *testing.T) {
	p, err := NextPort(nil, 0, func(port int) bool { return port != 8180 })
	if err != nil || p != 8181 {
		t.Fatalf("next-port-skips-bind-fail violated: p=%d err=%v", p, err)
	}
}

func TestNextPortNeverReusesTemplateEvenIfNotUsed(t *testing.T) {
	p, err := NextPort(nil, 8180, func(int) bool { return true })
	if err != nil || p == 8180 {
		t.Fatalf("next-port-never-reuses-template violated: p=%d err=%v", p, err)
	}
	if p != 8181 {
		t.Fatalf("next-port-never-reuses-template violated: want 8181 got %d", p)
	}
}

func TestNextPortExhausted(t *testing.T) {
	used := make([]int, 0, PortMax-PortMin+1)
	for p := PortMin; p <= PortMax; p++ {
		used = append(used, p)
	}
	_, err := NextPort(used, 0, func(int) bool { return true })
	if err == nil {
		t.Fatalf("next-port-exhausted violated: err=nil")
	}
}

func TestPortFromArgvSpaceAndEquals(t *testing.T) {
	if p := PortFromArgv([]string{"llama-server", "--port", "8193"}); p != 8193 {
		t.Fatalf("port-from-argv-space violated: %d", p)
	}
	if p := PortFromArgv([]string{"llama-server", "--port=8201"}); p != 8201 {
		t.Fatalf("port-from-argv-equals violated: %d", p)
	}
	if p := PortFromArgv([]string{"llama-server"}); p != 0 {
		t.Fatalf("port-from-argv-missing violated: %d", p)
	}
}
