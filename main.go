package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/mux"
	"gopkg.in/redis.v4"
)

type cacheConfig struct {
	Title      string
	Owner      ownerInfo
	Redis      map[string]redisInfo
	Kubernetes K8sInfo
	//	Test       map[string]testInfo
}

type ownerInfo struct {
	Name string
}

type redisInfo struct {
	Nodelabel  string
	MasterName string
	Port       int
	Password   string
	Db         int
}

type K8sInfo struct {
	Server string
	Port   int
}

func ReadCfg() cacheConfig {
	cfg_path := "/etc/fxqa-cache.conf"
	cfg := flag.String("cfg", "", "Configure file.")
	flag.Parse()
	if *cfg != "" {
		cfg_path = *cfg
	}
	var config cacheConfig
	fmt.Println(cfg_path)
	if _, err := toml.DecodeFile(cfg_path, &config); err != nil {
		log.Fatal(err)
	}
	fmt.Println(config)
	return config
}

var memprofile = flag.String("memprofile", "", "write memory profile to this file")

var router = mux.NewRouter()

type CacheRequestHandler struct {
	k8s_nodes map[string][]string
	// Master for write.
	master_clients  map[string]*redis.Client
	master_hashRing *Consistent

	// K8s Node.
	//node_hashRing *Consistent

	//// Slaver for read.
	//slaver_clients  map[string]*redis.Client
	//slaver_hashRing map[string]*Consistent
}

type ServerCFG struct {
	RedisIp string

	ListenPort int
}

func Info(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%v", "Cache Server Running...")
}

// k8s service port: 32457
func main() {
	cfg := ReadCfg()

	request_serv := new(CacheRequestHandler)
	err := request_serv.Init(cfg)
	if err != nil {
		fmt.Println("Redis Link Error.")
		return
	}

	//	go func() {
	//		log.Println(http.ListenAndServe("localhost:6060", nil))
	//		//http.ListenAndServe("localhost:6060", nil)
	//	}()
	router.HandleFunc("/info", Info).Methods("GET")

	router.HandleFunc("/key/{key}", request_serv.delKey).Methods("DELETE")
	router.HandleFunc("/key/{key}", request_serv.updateKey).Methods("PUT")
	router.HandleFunc("/key", request_serv.getKey).Methods("GET")

	router.HandleFunc("/string", request_serv.setString).Methods("POST")
	router.HandleFunc("/string/{key}", request_serv.updateString).Methods("GET")
	router.HandleFunc("/string", request_serv.getString).Methods("GET")

	// curl -d "key=test&v0 0 v1 1" /hash
	router.HandleFunc("/hash", request_serv.setHash).Methods("POST")
	router.HandleFunc("/hash", request_serv.getHash).Methods("GET")
	router.HandleFunc("/hash/{key:.*}", request_serv.updateHash).Methods("PUT")
	router.HandleFunc("/hash/{key}/{field}", request_serv.delHash).Methods("DELETE")

	router.HandleFunc("/set", request_serv.setSet).Methods("POST")
	router.HandleFunc("/set", request_serv.getSet).Methods("GET")
	router.HandleFunc("/set/{key0}/{key1}", request_serv.updateSet).Methods("PUT")
	router.HandleFunc("/set/{key0}", request_serv.updateSet).Methods("PUT")
	router.HandleFunc("/set/{key}", request_serv.delSet).Methods("DELETE")

	router.HandleFunc("/zset", request_serv.setZset).Methods("POST")
	router.HandleFunc("/zset", request_serv.getZset).Methods("GET")
	router.HandleFunc("/list", request_serv.setList).Methods("POST")

	// curl /hash?key=test | /hash?key=test&field=v0

	//	router.HandleFunc("/list", request_serv.getList).Methods("GET")
	//	router.HandleFunc("/data", request_serv.Get).Methods("GET")
	router.HandleFunc("/db", request_serv.RedisDBGet).Methods("GET")

	router.HandleFunc("/server", request_serv.ServerAdd).Methods("POST")
	router.HandleFunc("/server", request_serv.ServerGet).Methods("GET")

	//router.HandleFunc("/del", request_serv.del).Methods("DELETE")

	// Later.
	router.HandleFunc("/sync", request_serv.RedisSync).Methods("POST")

	http.Handle("/", router)
	http.ListenAndServe(":9090", nil)
}
