package main

import (
	"github.com/codegangsta/negroni"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/tokopedia/grace.v1"
)

type WebServer struct {
	Opt         Option
	router      *httprouter.Router
	listenErrCh chan error
}

type Option struct {
	Environment       string
	Port              string
	Domain            string
	TimestampHourDiff int `gcfg:"timestamp-hour-diff"`
}

type APIIntf interface {
	Register(*httprouter.Router)
}

func NewWeb(cfg *Option) *WebServer {
	router := httprouter.New()

	srv := &WebServer{
		Opt:         *cfg,
		router:      router,
		listenErrCh: make(chan error),
	}

	return srv
}

func (w *WebServer) RegisterAPI(api APIIntf) {
	api.Register(w.router)
}

func (w *WebServer) Run() {
	Println(nil, "[!!!] Starting Server at port:", w.Opt.Port)
	n := negroni.New()
	n.UseHandler(w.router)
	Fatalln("[!!!] Exiting gracefully... err: ", grace.Serve(w.Opt.Port, n))
}

func (w *WebServer) ListenError() <-chan error {
	return w.listenErrCh
}
