// Command fakepve serves a canned Proxmox VE API over self-signed HTTPS for
// demo recordings, without a hypervisor. Demo tooling only — never shipped.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"time"
)

const gb = 1 << 30

type route struct {
	pattern *regexp.Regexp
	payload string
}

func jsonData(inner string) string { return `{"data":` + inner + `}` }

func routes() []route {
	nodes := `[{"node":"pve1","status":"online","maxcpu":32,"maxmem":137438953472,"uptime":4200000,"type":"node"},{"node":"pve2","status":"online","maxcpu":24,"maxmem":103079215104,"uptime":4100000,"type":"node"}]`
	storage := fmt.Sprintf(`[{"storage":"local-lvm","type":"lvmthin","content":"images,rootdir","enabled":1,"active":1,"total":%d,"used":%d,"used_fraction":0.34},{"storage":"local","type":"dir","content":"iso,backup,vztmpl","enabled":1,"active":1,"total":%d,"used":%d,"used_fraction":0.29},{"storage":"tank","type":"zfspool","content":"images","enabled":1,"active":1,"total":%d,"used":%d,"used_fraction":0.29}]`,
		1800*gb, 620*gb, 200*gb, 58*gb, 7200*gb, 2100*gb)
	bridges := `[{"iface":"vmbr0","type":"bridge","active":1,"cidr":"192.168.1.2/24","method":"static"},{"iface":"vmbr1","type":"bridge","active":1,"cidr":"10.10.0.1/24","method":"static"}]`

	mk := func(p, payload string) route {
		return route{regexp.MustCompile(p), jsonData(payload)}
	}
	return []route{
		mk(`^POST /api2/json/access/ticket$`, `{"ticket":"PVE:root@pam:demo","CSRFPreventionToken":"demo:token","username":"root@pam"}`),
		mk(`^GET /api2/json/version$`, `{"version":"8.4.1","release":"8.4","repoid":"demo"}`),
		mk(`^GET /api2/json/nodes$`, nodes),
		mk(`^GET /api2/json/nodes/pve[12]/status$`, `{"uptime":4200000,"cpuinfo":{"cpus":32},"memory":{"total":137438953472}}`),
		mk(`^GET /api2/json/nodes/pve[12]/version$`, `{"version":"8.4.1","release":"8.4"}`),
		mk(`^GET /api2/json/nodes/pve[12]/storage/local/status$`, `{"storage":"local","type":"dir","content":"iso,backup,vztmpl","enabled":1,"active":1}`),
		mk(`^GET /api2/json/nodes/pve[12]/storage/local/content`, `[{"volid":"local:iso/fedora-coreos-live.x86_64.iso","content":"iso","size":838860800}]`),
		mk(`^GET /api2/json/nodes/pve[12]/storage`, storage),
		mk(`^GET /api2/json/nodes/pve[12]/network`, bridges),
	}
}

func selfSigned() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "pve.demo.local"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

func main() {
	table := routes()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		for _, rt := range table {
			if rt.pattern.MatchString(key) {
				fmt.Fprint(w, rt.payload)
				return
			}
		}
		log.Printf("unmatched: %q", key) //nolint:gosec // local-only demo stub; requests originate from the recording on 127.0.0.1
		fmt.Fprint(w, jsonData("null"))
	})

	cert, err := selfSigned()
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Addr:              "127.0.0.1:8006",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	log.Println("fake pve on https://127.0.0.1:8006")
	log.Fatal(srv.ListenAndServeTLS("", ""))
}
