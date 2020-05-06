package proxy

import (
	cryptoRand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"github.com/LilyPad/GoLilyPad/server/proxy/api"
	"github.com/LilyPad/GoLilyPad/server/proxy/connect"
	"io/ioutil"
	"net"
	"path/filepath"
	"plugin"
	"regexp"
	"strings"
	"time"
)

type Server struct {
	listener        net.Listener
	sessionRegistry *SessionRegistry

	apiContext  *apiContext
	apiEventBus *eventBus

	bind           *string
	motd           *string
	maxPlayers     *uint16
	syncMaxPlayers *bool
	authenticate   *bool
	regex          *regexp.Regexp
	router         Router
	localizer      Localizer
	connect        *connect.ProxyConnect
	privateKey     *rsa.PrivateKey
	publicKey      []byte
}

func NewServer(bind *string, motd *string, maxPlayers *uint16, syncMaxPlayers *bool, authenticate *bool, kickRegex *string, router Router, localizer Localizer, connect *connect.ProxyConnect) (this *Server, err error) {
	this = new(Server)
	this.sessionRegistry = NewSessionRegistry()
	this.apiContext = NewAPIContext(this)
	this.apiEventBus = NewEventBus()
	this.bind = bind
	this.motd = motd
	this.maxPlayers = maxPlayers
	this.syncMaxPlayers = syncMaxPlayers
	this.authenticate = authenticate
	this.regex = regexp.MustCompile(*kickRegex)
	this.router = router
	this.localizer = localizer
	this.connect = connect
	this.privateKey, err = rsa.GenerateKey(cryptoRand.Reader, 2048)
	if err != nil {
		return
	}
	this.publicKey, err = x509.MarshalPKIXPublicKey(&this.privateKey.PublicKey)
	if err != nil {
		return
	}
	connect.OnRedirect(func(serverName string, player string) {
		session := this.sessionRegistry.GetByName(player)
		if session == nil {
			return
		}
		server := connect.Server(serverName)
		if server == nil {
			return
		}
		session.Redirect(server)
	})
	this.loadPlugins()
	return
}

func (this *Server) ListenAndServe() (err error) {
	this.listener, err = net.Listen("tcp", *this.bind)
	if err != nil {
		return
	}
	var conn net.Conn
	for {
		conn, err = this.listener.Accept()
		if err != nil {
			if neterr, ok := err.(net.Error); ok && neterr.Temporary() {
				time.Sleep(time.Second)
				continue
			}
			return
		}
		go NewSession(this, conn).Serve()
	}
	this.Close()
	return
}

func (this *Server) loadPlugins() {
	var context api.Context
	context = this.apiContext
	fileDir := "./plugins/"
	files, err := ioutil.ReadDir(fileDir)
	if err != nil {
		return
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(file.Name(), ".so") {
			continue
		}
		pluginOpen, err := plugin.Open(filepath.Join(fileDir, file.Name()))
		if err != nil {
			fmt.Println("Plugin load error, file:", file.Name(), "error:", err)
			continue
		}
		pluginHandle, err := pluginOpen.Lookup("Plugin")
		if err != nil {
			fmt.Println("Plugin init error, file:", file.Name(), "error:", err)
			continue
		}
		pluginHandle.(api.Plugin).Init(context)
	}
}

func (this *Server) Close() {
	if this.listener != nil {
		this.listener.Close()
	}
}

func (this *Server) ListenAddr() string {
	return *this.bind
}

func (this *Server) Motd() string {
	return *this.motd
}

func (this *Server) MaxPlayers() uint16 {
	return *this.maxPlayers
}

func (this *Server) SyncMaxPlayers() bool {
	return *this.syncMaxPlayers
}

func (this *Server) Authenticate() bool {
	return *this.authenticate
}
