package turnserver

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
        "fmt"
        "strings"

	"github.com/pion/turn/v2"
	"github.com/pion/webrtc/v3"
)

const DEFAULT_PORT = 1194

var username string
var password string
var Address string

// Generalize a TCP/IP address & port pair. When the
// port is '0' - an `off' is assumed.
//
type UDPTCPAddr struct {
	IP   net.IP
	Port int
	Zone string // IPv6 scoped addressing zone
}

// General inside/outside address; where the two are 
// the same in the cannonical simple case; but, for example
// in a DMZ or when NAT-ting; the internal address is
// that what the turns server listens on (i.e bind()); whereas
// any turn:// URI's and so on are constructed with the 
// outside address. The term 'Paired' is taken from NAT.
//
type PairedAddr struct {
        exposedAddr UDPTCPAddr // Or Relay Address
        internalAddr UDPTCPAddr
}

func (addr PairedAddr) String() string {
        return fmt.Sprintf("%v/%v", addr.exposedAddr, addr.internalAddr);
}

func ResolveUDPTCPAddr(address string) (*UDPTCPAddr, error) {
	// We're doing a 'cheat' here; and rely on the UDP translator; as we know
        // that UDP/TCP are indentical with regard to port/addr structure.
        addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
        // Complete the cheat by just copying out the address details; but not the network familly
        r := UDPTCPAddr { addr.IP, addr.Port, addr.Zone }
	return &r, nil
}

func NewUDPTCPAddr(Address string) (*UDPTCPAddr, error) {
        if Address == "" || Address == "off" {
		return &UDPTCPAddr{}, nil
        }
        ad := Address
        if Address == "auto" {
                ad = fmt.Sprintf(":%v", DEFAULT_PORT)
        } else
        if strings.Index(ad,":") == -1 {
                ad = fmt.Sprintf("%v:%v", ad, DEFAULT_PORT)
        }
        addr, err := ResolveUDPTCPAddr(ad)
        if err != nil {
                return nil, err
        }
        return addr, nil
}

func (addr UDPTCPAddr) String() string {
        if (addr.IP == nil) {
            return fmt.Sprintf("*:%v", addr.Port)
        }
        return fmt.Sprintf("%v:%v", addr.IP, addr.Port)
}

func NewPairedAddr(str string) (*PairedAddr, error) {
        i := strings.Index(str,"/")
        left :=str
        if i > 1 {
	     left = str[0:i]
             str = str[i+1:]
        }
        exposedAddr, err := NewUDPTCPAddr(left)
        if err != nil {
                return nil, err
        }
        internalAddr, err := NewUDPTCPAddr(str)
        if err != nil {
                return nil, err
        }

        e := PairedAddr { *exposedAddr, *internalAddr }
        return &e, nil
}

var server struct {
	mu        sync.Mutex
	addresses []net.Addr
	server    *turn.Server
}

// Remove any RFC 1918 addreses from a list of addresses.
func publicAddresses() ([]net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	var as []net.IP

	for _, addr := range addrs {
		switch addr := addr.(type) {
		case *net.IPNet:
			a := addr.IP.To4()
			if a == nil {
				continue
			}
			if !a.IsGlobalUnicast() {
				continue
			}
			if a[0] == 10 ||
				a[0] == 172 && a[1] >= 16 && a[1] < 32 ||
				a[0] == 192 && a[1] == 168 {
				continue
			}
			as = append(as, a)
		}
	}
	return as, nil
}

func listener(a net.IP, port int, relay net.IP) (*turn.PacketConnConfig, *turn.ListenerConfig) {
	var pcc *turn.PacketConnConfig
	var lc *turn.ListenerConfig
        as := a.String()
        ad := a.String()
        if a == nil { as = ""; ad = "*" }
	s := net.JoinHostPort(as, strconv.Itoa(port))

	var g turn.RelayAddressGenerator 
	raddr := a.String()
	if relay == nil || relay.IsUnspecified() {
		g = &turn.RelayAddressGeneratorNone{
			Address: a.String(),
		}
	} else {
                raddr = relay.String()
		g = &turn.RelayAddressGeneratorStatic{
			RelayAddress: relay,
			Address:      a.String(),
		}
	}

	p, err := net.ListenPacket("udp", s)
	if err == nil {
		pcc = &turn.PacketConnConfig{
			PacketConn:            p,
			RelayAddressGenerator: g,
		}
		log.Printf("TURN: listener on udp:%v:%v, visible address: %v",ad,port,raddr)
	} else {
		log.Printf("TURN: listenPacket(%v): %v", s, err)
	}

	l, err := net.Listen("tcp", s)
	if err == nil {
		lc = &turn.ListenerConfig{
			Listener:              l,
			RelayAddressGenerator: g,
		}
		log.Printf("TURN: listener on tcp:%v:%v, visible address: %v",ad,port,raddr)
	} else {
		log.Printf("TURN: listen(%v): %v", s, err)
	}

	return pcc, lc
}

func Start() error {
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.server != nil {
		return nil
	}
        addressPair, err := NewPairedAddr(Address)
        if err != nil {
		return errors.New(fmt.Sprintf("TURN: Address error: %v", err))
        }
	if addressPair.internalAddr.Port == 0 {
		return errors.New("TURN: built-in TURN server disabled")
	}
	username = "galene"
	buf := make([]byte, 6)
	_, err = rand.Read(buf)
	if err != nil {
		return err
	}

	buf2 := make([]byte, 8)
	base64.RawStdEncoding.Encode(buf2, buf)
	password = string(buf2)

	var lcs []turn.ListenerConfig
	var pccs []turn.PacketConnConfig
	if addressPair.exposedAddr.IP != nil && !addressPair.exposedAddr.IP.IsUnspecified() {
		a := addressPair.exposedAddr.IP.To4()
		if a == nil {
			return errors.New("couldn't parse address/not an IPv4 address")
		}
		pcc, lc := listener(addressPair.internalAddr.IP, addressPair.internalAddr.Port, addressPair.exposedAddr.IP)
		if pcc != nil {
			pccs = append(pccs, *pcc)
			server.addresses = append(server.addresses, &net.UDPAddr{
				IP:   addressPair.exposedAddr.IP,
				Port: addressPair.exposedAddr.Port,
			})
			log.Printf("TURN: External address udp:%v", addressPair.exposedAddr)
		}
		if lc != nil {
			lcs = append(lcs, *lc)
			server.addresses = append(server.addresses, &net.TCPAddr{
				IP:   addressPair.exposedAddr.IP,
				Port: addressPair.exposedAddr.Port,
			})
			log.Printf("TURN: External address tcp:%v", addressPair.exposedAddr)
		}
	} else {
		as, err := publicAddresses()
		if err != nil {
			return err
		}

		if len(as) == 0 {
			return errors.New("no public addresses")
		}

		for _, a := range as {
			pcc, lc := listener(a, addressPair.internalAddr.Port, nil)
			if pcc != nil {
				pccs = append(pccs, *pcc)
				server.addresses = append(server.addresses,
					&net.UDPAddr{
						IP:   a,
						Port: addressPair.exposedAddr.Port,
					},
				)
				log.Printf("TURN: external address udp:%v:%v", a, addressPair.exposedAddr.Port)
			}
			if lc != nil {
				lcs = append(lcs, *lc)
				server.addresses = append(server.addresses,
					&net.TCPAddr{
						IP:   a,
						Port: addressPair.exposedAddr.Port,
					},
				)
				log.Printf("TURN: external address tcp:%v:%v", a, addressPair.exposedAddr.Port)
			}
		}
	}

	if len(pccs) == 0 && len(lcs) == 0 {
		return errors.New("couldn't establish any listeners")
	}
	log.Printf("TURN: Starting built-in TURN server")

	server.server, err = turn.NewServer(turn.ServerConfig{
		Realm: "galene.org",
		AuthHandler: func(u, r string, src net.Addr) ([]byte, bool) {
			if u != username || r != "galene.org" {
				return nil, false
			}
			return turn.GenerateAuthKey(u, r, password), true
		},
		ListenerConfigs:   lcs,
		PacketConnConfigs: pccs,
	})

	if err != nil {
		server.addresses = nil
		return err
	}

	return nil
}

func ICEServers() []webrtc.ICEServer {
	server.mu.Lock()
	defer server.mu.Unlock()

	if len(server.addresses) == 0 {
		return nil
	}

	var urls []string
	for _, a := range server.addresses {
		switch a := a.(type) {
		case *net.UDPAddr:
			urls = append(urls, "turn:"+a.String())
		case *net.TCPAddr:
			urls = append(urls, "turn:"+a.String()+"?transport=tcp")
		default:
			log.Printf("unexpected TURN address %T", a)
		}
	}

	return []webrtc.ICEServer{
		{
			URLs:       urls,
			Username:   username,
			Credential: password,
		},
	}

}

func Stop() error {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.addresses = nil
	if server.server == nil {
		return nil
	}
	log.Printf("Stopping built-in TURN server")
	err := server.server.Close()
	server.server = nil
	return err
}

func StartStop(start bool) error {
	if Address == "auto" {
		if start {
			return Start()
		}
		return Stop()
	} else if Address == "" {
		return Stop()
	}
	return Start()
}
