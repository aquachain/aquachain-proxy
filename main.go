package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/aquachain/aquachain-proxy/proxy"

	"github.com/goji/httpauth"
	"github.com/gorilla/mux"
	"github.com/yvasiyarov/gorelic"
)

var cfg proxy.Config

//go:embed aquaproxy.example.json www/*.* www/*/*.*
var embedfs embed.FS

var mainserver *proxy.ProxyServer
var htserver *http.Server
var defaultConfigname = "aquaproxy.json"
var cfgfilename string

func init() {
	// ensure '-mkcfg' flag will work
	cfgfile, err := embedfs.ReadFile("aquaproxy.example.json")
	if err != nil {
		panic(err.Error())
	}
	if err := json.Unmarshal(cfgfile, &cfg); err != nil {
		panic(err.Error())
	}

	// read config from file
	var filenames = []string{"aquaproxy.json", "/etc/aquaproxy.json", "/opt/aquaproxy/aquaproxy.json"}
	for _, filename := range filenames {
		if _, err := os.Stat(filename); err == nil {
			log.Printf("Reading INIT config from %v", filename)
			if err := readConfig(filename, &cfg); err != nil {
				log.Fatalf("Error reading config %q: %v", filename, err)
			}
			cfgfilename = filename
			return
		}
	}
}

func startProxy() error {
	if cfg.Threads > 0 {
		runtime.GOMAXPROCS(cfg.Threads)
		log.Printf("Running with %v threads", cfg.Threads)
	} else {
		n := runtime.NumCPU()
		runtime.GOMAXPROCS(n)
		log.Printf("Running with default %v threads", n)
	}

	r := mux.NewRouter()
	s, err := proxy.NewEndpoint(&cfg)
	if err != nil {
		return err
	}
	mainserver = s

	if cfg.Frontend.Listen != "" {
		log.Printf("Starting frontend on %v", cfg.Frontend.Listen)
		go startFrontend(&cfg, s)
	}

	r.Handle("/{id:.+}", s).Methods("POST")
	r.Handle("/0x{unused:.+}/{id:.+}", s).Methods("POST")
	r.Handle("/{diff:.+}/{id:.+}", s).Methods("POST")
	log.Printf("Starting proxy on %v", cfg.Proxy.Listen)
	htserver = &http.Server{Addr: cfg.Proxy.Listen, Handler: r}
	go func() {
		ch := make(chan os.Signal, 10)
		signal.Notify(ch, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGUSR1)
		for s.Context.Err() == nil {
			sig := <-ch
			switch sig {
			case syscall.SIGUSR1:
				log.Println("Reloading config")
				if err := readConfig(cfgfilename, &cfg); err != nil {
					log.Println("Error reloading config:", err)
				} else {
					log.Println("Config reloaded")
				}
			default:
				s.Cancel(fmt.Errorf("signal: %v", sig))
			}
		}
		<-s.Context.Done()
		htserver.Close()
		frontendserver.Close()
	}()

	return htserver.ListenAndServe()
	//return http.ListenAndServe(cfg.Proxy.Listen, r)
}

func startFrontend(cfg *proxy.Config, s *proxy.ProxyServer) {

	r := mux.NewRouter()
	r.HandleFunc("/stats", s.StatsIndex)
	if _, err := os.Stat("www/index.html"); err == nil && !cfg.Frontend.ForceEmbed {
		log.Printf("Using disk filesystem for frontend (rename www dir to use embedded, or use -e flag)")
		r.PathPrefix("/").Handler(http.FileServer(http.Dir("www")))
	} else {
		wwdir, err := fs.Sub(embedfs, "www")
		if err != nil {
			panic(err.Error)
		}
		f, err := wwdir.Open("index.html")
		if err == nil {
			f.Close()
		} else {
			panic(err.Error())
		}

		log.Printf("Using embedded filesystem for frontend (create www dir to use disk)")
		r.PathPrefix("/").Handler(http.FileServer(http.FS(wwdir)))
	}
	frontendserver = &http.Server{Addr: cfg.Frontend.Listen, Handler: r}
	if len(cfg.Frontend.Password) > 0 {
		frontendserver.Handler = httpauth.SimpleBasicAuth(cfg.Frontend.Login, cfg.Frontend.Password)(r)
	}
	err := http.ListenAndServe(cfg.Frontend.Listen, r)
	if mainserver.Context.Err() == nil {
		mainserver.Cancel(err)
	}
	log.Printf("frontendserver closed: %v", err)
}

var frontendserver *http.Server

func startNewrelic() {
	if cfg.NewrelicEnabled && cfg.NewrelicKey != "" && cfg.NewrelicName != "" {
		nr := gorelic.NewAgent()
		nr.Verbose = cfg.NewrelicVerbose
		nr.NewrelicLicense = cfg.NewrelicKey
		nr.NewrelicName = cfg.NewrelicName
		nr.Run()
	}
}

func readConfig(configFileName string, cfg *proxy.Config) error {
	log.Printf("Loading config: %v", configFileName)
	configFile, err := os.Open(configFileName)
	if err != nil {
		return fmt.Errorf("config file error: %w", err)
	}
	defer configFile.Close()
	jsonParser := json.NewDecoder(configFile)
	if err = jsonParser.Decode(&cfg); err != nil {
		return fmt.Errorf("config syntax error: %w", err)
	}
	return nil
}

func main() {
	var mkcfg bool
	var alwaysembed bool
	flag.BoolVar(&mkcfg, "mkcfg", false, "create config file (may be combined with -cfg)")
	flag.StringVar(&cfgfilename, "cfg", defaultConfigname, "path to config file (json, use -mkcfg to create)")
	flag.BoolVar(&alwaysembed, "e", alwaysembed, "always use embedded filesystem")

	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}
	cfgfile := cfgfilename
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	if flag.NArg() == 1 && cfgfile == defaultConfigname {
		cfgfile = flag.Arg(0)
	}
	if mkcfg {
		if _, err := os.Stat(cfgfile); err == nil {
			println("fatal: config file already exists")
			os.Exit(1)
		}
		if err := mkconfig(cfgfile); err != nil {
			println("fatal: creating config file:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if cfgfilename != defaultConfigname || len(cfg.Upstream) == 0 {
		log.Printf("reading alt-config from %v", cfgfilename)
		readConfig(cfgfile, &cfg)
	}
	startNewrelic()
	startProxy()
	log.Printf("fatal: %+v", context.Cause(mainserver.Context))
}

func mkconfig(cfgfile string) error {
	f, err := os.Create(cfgfile)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	return enc.Encode(cfg)
}
