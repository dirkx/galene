package turnserver

import (
	"testing"
)

func TestParseAddr(t *testing.T) {
	a := []struct{ p, g string }{
		{"", 					"{<nil> 0 }/{<nil> 0 }"},
		{"off", 				"{<nil> 0 }/{<nil> 0 }"},
		{"auto",				"{<nil> 1194 }/{<nil> 1194 }"},
		{":1234", 				"{<nil> 1234 }/{<nil> 1234 }"},
		{":1234/:4321", 			"{<nil> 1234 }/{<nil> 4321 }"},
		{"10.11.0.1:1234", 			"{10.11.0.1 1234 }/{10.11.0.1 1234 }"},
		{"10.11.0.1:1234/:4321", 		"{10.11.0.1 1234 }/{<nil> 4321 }"},
		{"10.11.0.1:1234/1.2.3.4:4321", 	"{10.11.0.1 1234 }/{1.2.3.4 4321 }"},
		{"always-1-2-3-4.webweaving.org", 	"{1.2.3.4 1194 }/{1.2.3.4 1194 }"},
		{"always-1-2-3-4.webweaving.org:4321", 	"{1.2.3.4 4321 }/{1.2.3.4 4321 }"},
		{"always-1-2-3-4.webweaving.org:4321/:1234", 	"{1.2.3.4 4321 }/{<nil> 1234 }"},
		{"always-1-2-3-4.webweaving.org:4321/127.0.0.1:1234", 	"{1.2.3.4 4321 }/{127.0.0.1 1234 }"},
		{"always-1-2-3-4.webweaving.org:4321/127.0.0.1", 	"{1.2.3.4 4321 }/{127.0.0.1 1194 }"},
/* Only works on an pure IPv4 machine
		{"localhost:1234/1.2.3.4:4321", 	"{127.0.0.1 1234 }/{1.2.3.4 4321 }"},
		{"always-1-2-3-4.webweaving.org:4321/localhost", 	"{1.2.3.4 4321 }/{127.0.0.1 1194 }"},
		{"always-1-2-3-4.webweaving.org:4321/localhost:1234", 	"{1.2.3.4 4321 }/{127.0.0.1 1234 }"},
*/
	}

	for _, pg := range a {
		g, err := NewPairedAddr(pg.p)
		if err != nil {
			t.Errorf("Error: '%v' (not expected)", err)
		}
		if g.String() != pg.g {
			t.Errorf("'%v', got '%v', expected '%v'",
				pg.p, g, pg.g)
		}
	}
}
