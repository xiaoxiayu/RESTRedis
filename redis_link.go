package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/redis.v4"
)

func (this *CacheRequestHandler) GetFormValue(w http.ResponseWriter, r *http.Request, form_name string) string {
	defer func() string {
		if err := recover(); err != nil {
			//fmt.Println(err)
			//fmt.Fprintf(w, "%s", err)
			return ""
		}
		return r.Form[form_name][0]
	}()
	return r.Form[form_name][0]
}

func (this *CacheRequestHandler) setExpire(key, exp string, redis_client *redis.Client) error {
	exp_int, err := strconv.Atoi(exp)
	if err != nil {
		return err
	}

	err = redis_client.Expire(key, (time.Duration)(exp_int)).Err()
	if err != nil {
		return err
	}
	return nil
}

func (this *CacheRequestHandler) addServer(name string, redis_cfg redisInfo) error {
	master_client := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    redis_cfg.MasterName,
		SentinelAddrs: this.k8s_nodes[name],
		DB:            redis_cfg.Db, // use default DB
	})

	_, err := master_client.Ping().Result()
	if err == nil {
		fmt.Println("Redis Link Success:", this.k8s_nodes)
		this.master_clients[name] = master_client
		this.master_hashRing.Add(name)
		return nil
	} else {
		fmt.Println("Redis Link Failed:", this.k8s_nodes)
	}

	return fmt.Errorf("Redis Link ERROR.")
}

func (this *CacheRequestHandler) Init(cfg cacheConfig) error {
	this.master_clients = make(map[string]*redis.Client)
	this.k8s_nodes = make(map[string][]string)
	this.master_hashRing = NewConsisten()

	for name, redis_cfg := range cfg.Redis {
		nodes, err := GetNodes("http://"+cfg.Kubernetes.Server+":"+strconv.Itoa(cfg.Kubernetes.Port)+"/api/v1/nodes", redis_cfg.Nodelabel)
		if err != nil {
			log.Fatalf("Get Nodes Error:%s, %s", name, err.Error())
			return err
		}

		for _, node := range nodes {
			this.k8s_nodes[name] = append(this.k8s_nodes[name], node+":"+strconv.Itoa(cfg.Redis[name].Port))
		}
		err = this.addServer(name, redis_cfg)
		if err != nil {
			log.Println(name, ",ERROR: ", err.Error())
		}
	}
	return nil
}

func (this *CacheRequestHandler) setString(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: key Empty")
		return
	}
	val := this.GetFormValue(w, r, "value")
	if val == "" {
		fmt.Fprintf(w, "%v", "ERR: val Empty")
		return
	}
	exp := this.GetFormValue(w, r, "expire")

	name := this.master_hashRing.Get(key)

	err := this.master_clients[name].Set(key, val, 0).Err()
	if err != nil {
		fmt.Fprintf(w, "%v", "ERR: "+err.Error())
	}

	if exp != "" {
		err := this.setExpire(key, exp, this.master_clients[name])
		if err != nil {
			fmt.Fprintf(w, "%v", "ERR: "+err.Error())
			return
		}
	}

	fmt.Fprintf(w, "%v", "0")
}

func (this *CacheRequestHandler) updateString(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	vars := mux.Vars(r)
	key := vars["key"]
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: key Empty")
		return
	}
	val := this.GetFormValue(w, r, "value")
	if val == "" {
		fmt.Fprintf(w, "%v", "ERR: val Empty")
		return
	}
	exp := this.GetFormValue(w, r, "expire")

	name := this.master_hashRing.Get(key)

	err := this.master_clients[name].Set(key, val, 0).Err()
	if err != nil {
		fmt.Fprintf(w, "%v", "ERR: "+err.Error())
	}

	if exp != "" {
		err := this.setExpire(key, exp, this.master_clients[name])
		if err != nil {
			fmt.Fprintf(w, "%v", "ERR: "+err.Error())
			return
		}
	}

	fmt.Fprintf(w, "%v", "0")
}

func (this *CacheRequestHandler) setHash(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	var err error

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: key Empty")
		return
	}
	name := this.master_hashRing.Get(key)
	client := this.master_clients[name]

	fields := r.Form["field"]
	if fields == nil {
		ErrorParam(w, "field")
		return
	}

	vals := r.Form["value"]
	if vals == nil {
		ErrorParam(w, "value")
		return
	}

	if len(fields) != len(vals) {
		ErrorParam(w, "field and value diff")
		return
	}

	if len(fields) >= 2 {
		val_map, err := ParseHashValue(fields, vals)
		fmt.Println(val_map)
		if err != nil {
			ErrorExcu(w, err)
			return
		}
		err = client.HMSet(key, val_map).Err()
	} else {
		err = client.HSet(key, fields[0], vals[0]).Err()
	}
	if err != nil {
		ErrorExcu(w, err)
		return
	}

	exp := this.GetFormValue(w, r, "expire")
	if exp != "" {
		err := this.setExpire(key, exp, this.master_clients[name])
		if err != nil {
			ErrorExcu(w, err)
			return
		}
	}

	ErrorNil(w, nil)
}

func (this *CacheRequestHandler) setList(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: key Empty")
		return
	}
	val := this.GetFormValue(w, r, "value")
	if val == "" {
		fmt.Fprintf(w, "%v", "ERR: val Empty")
		return
	}

	port := this.master_hashRing.Get(key)
	err := this.master_clients[port].LPush(key, val).Err()
	if err != nil {
		fmt.Fprintf(w, "%v", "ERR: "+err.Error())
	}

	exp := this.GetFormValue(w, r, "expire")
	if exp != "" {
		err := this.setExpire(key, exp, this.master_clients[port])
		if err != nil {
			fmt.Fprintf(w, "%v", "ERR: "+err.Error())
			return
		}
	}

	fmt.Fprintf(w, "%v", "0")
}

func (this *CacheRequestHandler) setZset(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: key Empty")
		return
	}
	val := this.GetFormValue(w, r, "value")
	if val == "" {
		fmt.Fprintf(w, "%v", "ERR: member Empty")
		return
	}

	port := this.master_hashRing.Get(key)
	val_zset, err := ParseZSetValue(val)
	err = this.master_clients[port].ZAdd(key, val_zset).Err()
	if err != nil {
		fmt.Fprintf(w, "%v", "ERR: "+err.Error())
	}

	exp := this.GetFormValue(w, r, "expire")
	if exp != "" {
		err := this.setExpire(key, exp, this.master_clients[port])
		if err != nil {
			fmt.Fprintf(w, "%v", "ERR: "+err.Error())
			return
		}
	}

	fmt.Fprintf(w, "%v", "0")
}

func (this *CacheRequestHandler) getZset(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: key Empty")
		return
	}

	action_type := this.GetFormValue(w, r, "type")
	if action_type == "" {
		fmt.Fprintf(w, "%v", "ERR: key Empty")
		return
	}

	member := this.GetFormValue(w, r, "member")
	if member == "" {
		fmt.Fprintf(w, "%v", "ERR: member Empty")
		return
	}

	name := this.master_hashRing.Get(key)
	client := this.master_clients[name]
	if action_type == "zrank" {
		val, err := client.ZRank(key, member).Result()
		if err != nil {
			ErrorExcu(w, err)
		} else {
			ErrorNil(w, val)
		}
		return
	} else if action_type == "zrevrank" {
		val, err := client.ZRevRank(key, member).Result()
		if err != nil {
			ErrorExcu(w, err)
		} else {
			ErrorNil(w, val)
		}
		return
	} else if action_type == "zrange" {
		zrange_start := this.GetFormValue(w, r, "start")
		if zrange_start == "" {
			ErrorParam(w, "start")
			return
		}
		zrange_end := this.GetFormValue(w, r, "end")
		if zrange_end == "" {
			ErrorParam(w, "end")
			return
		}
		zrange_s, err := strconv.ParseInt(zrange_start, 10, 64)
		if err != nil {
			ErrorParam(w, "start")
			return
		}
		zrange_e, err := strconv.ParseInt(zrange_end, 10, 64)
		if err != nil {
			ErrorParam(w, "end")
			return
		}

		vals, err := client.ZRange(key, zrange_s, zrange_e).Result()
		if err != nil {
			ErrorExcu(w, err)
		} else {
			ErrorNil(w, vals)
		}
		return
	} else if action_type == "zrevrange" {

	} else if action_type == "zrangebyscore" {

	} else if action_type == "zcard" {

	} else if action_type == "zscore" {

	}
}

func (this *CacheRequestHandler) getString(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: Key Empty")
	}

	port := this.master_hashRing.Get(key)
	//	slaver_ip := this.slaver_hashRing[master_ip].Get(key)
	val, err := this.master_clients[port].Get(key).Result()
	if err == redis.Nil {
		fmt.Fprintf(w, "%v", "NIL")
	} else if err != nil {
		ErrorExcu(w, err)
	} else {
		ErrorNil(w, val)
	}
}

func (this *CacheRequestHandler) getHash(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", `{"_type":"1", "_msg":"key empty"}`)
		return
	}

	port := this.master_hashRing.Get(key)
	client := this.master_clients[port]

	action_type := this.GetFormValue(w, r, "type")

	if action_type == "hget" {
		field := this.GetFormValue(w, r, "field")
		if field != "" {
			val, err := client.HGet(key, field).Result()
			if err == redis.Nil {
				fmt.Fprintf(w, "%v", "NIL")
			} else if err != nil {
				fmt.Fprintf(w, "%v", "ERR:"+err.Error())
				return
			}
			ErrorNil(w, val)
			return
		}
	} else if action_type == "hmget" {
		fields := r.Form["field"]
		if fields == nil {
			ErrorParam(w, "field")
			return
		}

		vals, err := client.HMGet(key, fields...).Result()
		if err == redis.Nil {
			fmt.Fprintf(w, "%v", "NIL")
		} else if err != nil {
			fmt.Fprintf(w, "%v", "ERR:"+err.Error())
		}
		if len(vals) == 0 {
			fmt.Fprintf(w, "%v", `{"_type":"0","val":[]}`)
			return
		}
		vals_str := "["
		for _, s := range vals {
			vals_str += `"` + s.(string) + `",`
		}
		vals_str = vals_str[:len(vals_str)-1] + `]`
		ret := `{"_type":"0","val":` + vals_str + `}`
		fmt.Fprintf(w, "%v", ret)
		return
	}

	//	slaver_ip := this.slaver_hashRing[master_ip].Get(key)
	val, err := client.HGetAll(key).Result()
	if err == redis.Nil {
		fmt.Fprintf(w, "%v", "NIL")
	} else if err != nil {
		fmt.Fprintf(w, "%v", "ERR:"+err.Error())
	}
	if len(val) == 0 {
		val["_type"] = "3"
	} else {
		val["_type"] = "0"
	}

	ret, _ := json.Marshal(val)

	fmt.Fprintf(w, "%v", string(ret))
}

func (this *CacheRequestHandler) updateHash(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	var err error
	vars := mux.Vars(r)
	key := vars["key"]
	if key == "" {
		ErrorNil(w, "key")
		return
	}
	name := this.master_hashRing.Get(key)
	client := this.master_clients[name]

	fields := r.Form["field"]
	if fields == nil {
		ErrorParam(w, "field")
		return
	}

	vals := r.Form["value"]
	if vals == nil {
		ErrorParam(w, "value")
		return
	}

	if len(vals) != len(fields) {
		ErrorParam(w, "value and field diff")
		return
	}

	if len(fields) >= 2 {
		val_map, err := ParseHashValue(fields, vals)
		if err != nil {
			ErrorExcu(w, err)
			return
		}
		err = client.HMSet(key, val_map).Err()
	} else {
		err = client.HSet(key, fields[0], vals[0]).Err()
		if err != nil {
			ErrorExcu(w, err)
			return
		}
	}

	exp := this.GetFormValue(w, r, "expire")
	if exp != "" {
		err := this.setExpire(key, exp, client)
		if err != nil {
			ErrorExcu(w, err)
			return
		}
	}
	ErrorNil(w, nil)
}

func (this *CacheRequestHandler) delHash(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	vars := mux.Vars(r)
	key := vars["key"]

	port := this.master_hashRing.Get(key)
	//	slaver_ip := this.slaver_hashRing[master_ip].Get(key)
	client := this.master_clients[port]

	field := vars["field"]
	vals := strings.Split(field, " ")

	if len(vals) > 0 {
		delvals, err := client.HDel(key, vals...).Result()
		if err == redis.Nil {
			fmt.Fprintf(w, "%v", `{"_type":"1","_msg":"key is not exist"}`)
		} else if err != nil {
			fmt.Fprintf(w, "%v", fmt.Sprintf(`{"_type":"-1","_msg":"%s"}`, err.Error()))
		}
		fmt.Fprintf(w, "%v", fmt.Sprintf(`{"_type":"0","_msg":"%d"}`, delvals))
		return
	}
	fmt.Fprintf(w, "%v", `{"_type":"1","_msg":"field is null"}`)
}

func (this *CacheRequestHandler) setSet(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		ErrorParam(w, "key")
		return
	}

	members := r.Form["member"]
	if members == nil {
		ErrorParam(w, "members")
		return
	}

	new_vals := make([]interface{}, len(members))
	for i, v := range members {
		new_vals[i] = v
	}

	name := this.master_hashRing.Get(key)
	client := this.master_clients[name]

	err := client.SAdd(key, new_vals...).Err()
	if err != nil {
		ErrorExcu(w, err)
		return
	}

	exp := this.GetFormValue(w, r, "expire")
	if exp != "" {
		err := this.setExpire(key, exp, client)
		if err != nil {
			ErrorExcu(w, err)
			return
		}
	}

	ErrorNil(w, nil)
}

func (this *CacheRequestHandler) getSet(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		fmt.Fprintf(w, "%v", "ERR: Key Empty")
	}

	action_type := this.GetFormValue(w, r, "type")

	port := this.master_hashRing.Get(key)
	//	slaver_ip := this.slaver_hashRing[master_ip].Get(key)
	client := this.master_clients[port]

	if action_type == "srandmember" {
		val, err := client.SRandMember(key).Result()
		if err == redis.Nil {
			fmt.Fprintf(w, "%v", "NIL")
		} else if err != nil {
			fmt.Fprintf(w, "%v", "ERR:"+err.Error())
		}

		ret := `{"_type":"0","val":"` + val + `"}`
		fmt.Fprintf(w, "%v", ret)
		return
	} else if action_type == "scard" {
		val, err := client.SCard(key).Result()
		if err == redis.Nil {
			ErrorValNone(w)
			return
		} else if err != nil {
			ErrorExcu(w, err)
			return
		}

		fmt.Println(val)

		ErrorNil(w, val)
		return
	} else if action_type == "sismember" {
		fmt.Println("SISMEMBER")
		mem := this.GetFormValue(w, r, "member")
		if mem == "" {
			ErrorParam(w, "member")
			return
		}
		val, err := client.SIsMember(key, mem).Result()
		if err == redis.Nil {
			ErrorValNone(w)
			return
		} else if err != nil {
			ErrorExcu(w, err)
			return
		}
		fmt.Println("=========:", val)
		ErrorNil(w, val)
		return
	} else { // smembers
		vals, err := client.SMembers(key).Result()
		if err == redis.Nil {
			fmt.Fprintf(w, "%v", "NIL")
		} else if err != nil {
			fmt.Fprintf(w, "%v", "ERR:"+err.Error())
		}
		if len(vals) == 0 {
			fmt.Fprintf(w, "%v", `{"_type":"0","val":[]}`)
			return
		}
		vals_str := "["
		for _, s := range vals {
			vals_str += `"` + s + `",`
		}
		vals_str = vals_str[:len(vals_str)-1] + `]`
		ret := `{"_type":"0","val":` + vals_str + `}`
		fmt.Fprintf(w, "%v", ret)
		return
	}

}

func (this *CacheRequestHandler) updateSet(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	vars := mux.Vars(r)
	key := vars["key0"]

	name := this.master_hashRing.Get(key)
	//	slaver_ip := this.slaver_hashRing[master_ip].Get(key)
	client := this.master_clients[name]

	action_type := this.GetFormValue(w, r, "type")
	if action_type == "" {
		ErrorParam(w, `type`)
		return
	}

	if action_type == "sadd" {
		members := r.Form["member"]
		if members == nil {
			ErrorParam(w, `member`)
			return
		}

		new_vals := make([]interface{}, len(members))
		for i, v := range members {
			new_vals[i] = v
		}

		err := client.SAdd(key, new_vals...).Err()
		if err != nil {
			ErrorExcu(w, err)
			return
		}
	} else if action_type == "smove" {
		key_desc := vars["key1"]
		member := this.GetFormValue(w, r, "member")

		if key_desc == "" || member == "" {
			ErrorParam(w, `'key' or 'member'`)
			return
		}

		err := client.SMove(key, key_desc, member).Err()
		if err != nil {
			ErrorExcu(w, err)
			return
		}
	} else if action_type == "spop" {
		val, err := client.SPop(key).Result()
		if err != nil {
			ErrorExcu(w, err)
			return
		}
		ErrorNil(w, val)
		return
	} else if action_type == "srem" {
		members := r.Form["member"]
		if members == nil {
			ErrorParam(w, "member")
			return
		}

		new_vals := make([]interface{}, len(members))
		for i, v := range members {
			new_vals[i] = v
		}
		rem_cnt, err := client.SRem(key, new_vals...).Result()
		if err != nil {
			ErrorExcu(w, err)
			return
		}
		ErrorNil(w, rem_cnt)
		return
	}
}

func (this *CacheRequestHandler) delSet(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	vars := mux.Vars(r)
	key := vars["key"]

	name := this.master_hashRing.Get(key)
	//	slaver_ip := this.slaver_hashRing[master_ip].Get(key)
	client := this.master_clients[name]

	err := client.Del(key).Err()
	if err != nil {
		ErrorExcu(w, err)
	} else {
		ErrorNil(w, "")
	}

}

func (this *CacheRequestHandler) ServerAdd(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	ports := this.GetFormValue(w, r, "ports")
	if ports == "" {
		fmt.Fprintf(w, "%v", "ERR: ip Empty")
	}

	apiserver := this.GetFormValue(w, r, "apiserver")
	if apiserver == "" {
		fmt.Fprintf(w, "%v", "ERR: apiserver Empty")
	}

	pwd := this.GetFormValue(w, r, "pwd")
	if pwd == "" {
		fmt.Fprintf(w, "%v", "ERR: pwd Empty")
	}

	//	redis_ips := strings.Split(ports, ";")

	//	err := this.addServer(redis_ips, apiserver)
	//	if err != nil {
	//		fmt.Fprintf(w, "%v", "Add Server Failed.")
	//	}
}

func (this *CacheRequestHandler) ServerGet(w http.ResponseWriter, r *http.Request) {
	master_cnt := len(this.master_clients)
	//slave_cnt := len(this.slaver_clients)
	fmt.Fprintf(w, "%v", fmt.Sprintf(`{"Redis Master Count":%d}`, master_cnt))
}

func (this *CacheRequestHandler) RedisDBGet(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	ip := this.GetFormValue(w, r, "ip")
	if ip == "" {
		fmt.Fprintf(w, "%v", "ERR: Key Empty")
	}
	val, err := this.master_clients[ip].DbSize().Result()
	if err == redis.Nil {
		fmt.Fprintf(w, "%v", "NIL")
	} else if err != nil {
		fmt.Fprintf(w, "%v", "ERR:"+err.Error())
	} else {
		fmt.Fprintf(w, "%v", val)
	}
}

func (this *CacheRequestHandler) RedisSync(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	src := this.GetFormValue(w, r, "src")
	if src == "" {
		fmt.Fprintf(w, "%v", "ERR: Key Empty")
	}
	des := this.GetFormValue(w, r, "des")
	if des == "" {
		fmt.Fprintf(w, "%v", "ERR: Key Empty")
	}
	fmt.Fprintf(w, "%v", `Unsupport`)
}

func (this *CacheRequestHandler) delKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	port := this.master_hashRing.Get(key)
	//	slaver_ip := this.slaver_hashRing[master_ip].Get(key)
	client := this.master_clients[port]

	err := client.Del(key).Err()
	if err != nil {
		fmt.Fprintf(w, "%v", fmt.Sprintf(`{"_type":"-1","_msg":%s}`, err.Error()))
	} else {
		fmt.Fprintf(w, "%v", `{"_type":"0"}`)
	}
}

func (this *CacheRequestHandler) updateKey(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	vars := mux.Vars(r)
	key := vars["key"]

	name := this.master_hashRing.Get(key)
	client := this.master_clients[name]

	exp := this.GetFormValue(w, r, "expire")
	if exp != "" {
		err := this.setExpire(key, exp, client)
		if err != nil {
			ErrorExcu(w, err)
			return
		}
	}
	ErrorNil(w, nil)
}

func (this *CacheRequestHandler) getKey(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	key := this.GetFormValue(w, r, "key")
	if key == "" {
		ErrorParam(w, "key")
		return
	}

	action_type := this.GetFormValue(w, r, "type")
	if action_type == "" {
		ErrorParam(w, "type")
		return
	}
	name := this.master_hashRing.Get(key)
	client := this.master_clients[name]

	if action_type == "exists" {
		val, err := client.Exists(key).Result()
		if err != nil {
			ErrorExcu(w, err)
			return
		}
		ErrorNil(w, val)
		return
	}

	ErrorNil(w, nil)
}
