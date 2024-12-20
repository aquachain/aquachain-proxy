package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"

	"github.com/aquachain/aquachain-proxy/rpc"
)

type ProxyServer struct {
	config          *Config
	miners          MinersMap
	blockTemplate   atomic.Value
	upstream        int32
	upstreams       []*rpc.RPCClient
	hashrateWindow  time.Duration
	timeout         time.Duration
	roundShares     int64
	blocksMu        sync.RWMutex
	blockStats      map[int64]float64
	luckWindow      int64
	luckLargeWindow int64
	Context         context.Context
	Cancel          context.CancelCauseFunc
}

type Session struct {
	enc *json.Encoder
	ip  string
}

const (
	MaxReqSize = 1 * 1024
)

func NewEndpoint(cfg *Config) (*ProxyServer, error) {
	proxy := &ProxyServer{config: cfg, blockStats: make(map[int64]float64)}

	proxy.upstreams = make([]*rpc.RPCClient, len(cfg.Upstream))
	for i, v := range cfg.Upstream {
		client, err := rpc.NewRPCClient(v.Name, v.Url, v.Timeout, v.Pool, cfg.HttpClient)
		if err != nil {
			return nil, err
		}
		proxy.upstreams[i] = client
		log.Printf("Upstream: %s => %s", v.Name, v.Url)
	}
	log.Printf("Default upstream: %s => %s", proxy.rpc().Name, proxy.rpc().Url)

	proxy.miners = NewMinersMap()

	timeout, _ := time.ParseDuration(cfg.Proxy.ClientTimeout)
	proxy.timeout = timeout

	hashrateWindow, _ := time.ParseDuration(cfg.Proxy.HashrateWindow)
	proxy.hashrateWindow = hashrateWindow

	luckWindow, _ := time.ParseDuration(cfg.Proxy.LuckWindow)
	proxy.luckWindow = int64(luckWindow / time.Millisecond)
	luckLargeWindow, _ := time.ParseDuration(cfg.Proxy.LargeLuckWindow)
	proxy.luckLargeWindow = int64(luckLargeWindow / time.Millisecond)

	proxy.blockTemplate.Store(&BlockTemplate{})
	proxy.fetchBlockTemplate()

	refreshIntv, _ := time.ParseDuration(cfg.Proxy.BlockRefreshInterval)
	refreshTimer := time.NewTimer(refreshIntv)
	log.Printf("Set block refresh every %v", refreshIntv)

	checkIntv, _ := time.ParseDuration(cfg.UpstreamCheckInterval)
	checkTimer := time.NewTimer(checkIntv)
	ctx, cancel := context.WithCancelCause(context.Background())
	proxy.Context = ctx
	proxy.Cancel = cancel
	go func() {
		for proxy.Context.Err() == nil {
			select {
			case <-proxy.Context.Done():
			case <-refreshTimer.C:
				proxy.fetchBlockTemplate()
				refreshTimer.Reset(refreshIntv)
			}
		}
		log.Printf("no longer refreshing block template")
	}()

	go func() {
		for proxy.Context.Err() == nil {
			select {
			case <-proxy.Context.Done():
			case <-checkTimer.C:
				proxy.checkUpstreams()
				checkTimer.Reset(checkIntv)
			}
		}
		log.Printf("no longer checking upstreams")
	}()

	return proxy, nil
}

func (s *ProxyServer) rpc() *rpc.RPCClient {
	i := atomic.LoadInt32(&s.upstream)
	return s.upstreams[i]
}

func (s *ProxyServer) checkUpstreams() {
	candidate := int32(0)
	backup := false

	for i, v := range s.upstreams {
		ok, err := v.Check()
		if err != nil {
			log.Printf("Upstream %v didn't pass check: %v", v.Name, err)
		}
		if ok && !backup {
			candidate = int32(i)
			backup = true
		}
	}

	if s.upstream != candidate {
		log.Printf("Switching to %v upstream", s.upstreams[candidate].Name)
		atomic.StoreInt32(&s.upstream, candidate)
	}
}

func (s *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if err := s.handleClient(w, r); err != nil {
		s.writeError(w, http.StatusBadRequest, "Bad request")
		return
	}
}

func (s *ProxyServer) handleClient(w http.ResponseWriter, r *http.Request) error {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	cs := &Session{ip: ip, enc: json.NewEncoder(w)}
	defer r.Body.Close()
	connbuff := bufio.NewReaderSize(r.Body, MaxReqSize)

	for {
		data, isPrefix, err := connbuff.ReadLine()
		if isPrefix {
			log.Printf("Socket flood detected")
			return errors.New("socket flood")
		} else if err == io.EOF {
			break
		}

		if len(data) > 1 {
			var req JSONRpcReq
			err = json.Unmarshal(data, &req)
			if err != nil {
				log.Printf("Malformed request: %v", err)
				return err
			}
			cs.handleMessage(s, r, &req)
		}
	}
	return nil
}

func (cs *Session) handleMessage(s *ProxyServer, r *http.Request, req *JSONRpcReq) {
	if req.Id == nil {
		log.Println("Missing RPC id")
		r.Close = true
		return
	}

	vars := mux.Vars(r)
	diff := vars["diff"]
	if diff == "" {
		diff = "0"
	}

	// Handle RPC methods
	switch req.Method {
	case "eth_getWork", "aqua_getWork":
		reply, errReply := s.handleGetWorkRPC(cs, diff, vars["id"])
		if errReply != nil {
			cs.sendError(req.Id, errReply)
			break
		}
		cs.sendResult(req.Id, &reply)
	case "eth_submitWork", "aqua_submitWork":
		var params []string
		err := json.Unmarshal(*req.Params, &params)
		if err != nil {
			log.Println("Unable to parse params")
			break
		}
		reply, errReply := s.handleSubmitRPC(cs, diff, vars["id"], params)
		if errReply != nil {
			err = cs.sendError(req.Id, errReply)
			break
		}
		cs.sendResult(req.Id, &reply)
	case "eth_submitHashrate", "aqua_submitHashrate": // doesnt do anything
		reply := true
		if s.config.Proxy.SubmitHashrate {
			reply = s.handleSubmitHashrate(cs, req)
		}
		cs.sendResult(req.Id, reply)
	default:
		errReply := s.handleUnknownRPC(cs, req)
		cs.sendError(req.Id, errReply)
	}
}

func (cs *Session) sendResult(id *json.RawMessage, result interface{}) error {
	message := JSONRpcResp{Id: id, Version: "2.0", Error: nil, Result: result}
	return cs.enc.Encode(&message)
}

func (cs *Session) sendError(id *json.RawMessage, reply *ErrorReply) error {
	message := JSONRpcResp{Id: id, Version: "2.0", Error: reply}
	return cs.enc.Encode(&message)
}

func (s *ProxyServer) writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
}

func (s *ProxyServer) currentBlockTemplate() *BlockTemplate {
	return s.blockTemplate.Load().(*BlockTemplate)
}

func (s *ProxyServer) registerMiner(miner *Miner) {
	s.miners.Set(miner.Id, miner)
}
