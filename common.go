package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"gopkg.in/redis.v4"
)

func ErrorExcu(w http.ResponseWriter, err error) {
	fmt.Fprintf(w, "%v", fmt.Sprintf(`{"_type":"-1","_msg":"%s"}`, err.Error()))
}
func ErrorValNone(w http.ResponseWriter) {
	fmt.Fprintf(w, "%v", `{"_type":"-1","_msg":"nil"}`)
}

func ErrorNil(w http.ResponseWriter, val interface{}) {
	if val == nil {
		fmt.Fprintf(w, "%v", `{"_type":"0", "_msg":"ok"}`)
		return
	}
	w_s := ""
	switch val.(type) {
	case int:
		w_s = `"` + strconv.Itoa(val.(int)) + `"`
	case int64:
		w_s = `"` + strconv.FormatInt(val.(int64), 10) + `"`
	case string:
		w_s = `"` + val.(string) + `"`
	case bool:
		if val.(bool) {
			w_s = "true"
		} else {
			w_s = "false"
		}
	case []string:
		w_s += "["
		for _, s := range val.([]string) {
			w_s += `"` + s + `",`
		}
		w_s += w_s[:len(w_s)-2]
		w_s += "]"
	}

	fmt.Fprintf(w, "%v", fmt.Sprintf(`{"_type":"0", "val":%s}`, w_s))
}

func ErrorParam(w http.ResponseWriter, param string) {
	fmt.Fprintf(w, "%v", fmt.Sprintf(`{"_type":"1", "_msg":"Param '%s' Error."}`, param))
}

func HTTPGet(url_str string) ([]byte, error) {
	client := http.Client{
		Timeout: 6000e9,
	}
	r, err := client.Get(url_str)
	if err != nil {
		return nil, err
	}

	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	return body, err
}

//func ParseHashValue(hash_val string) (map[string]string, int, error) {
//	val_map := make(map[string]string)
//	vals := strings.Split(hash_val, " ")
//	val_size := len(vals)
//	if val_size%2 != 0 {
//		return nil, -1, fmt.Errorf("Values len error.")
//	}
//	val_key := ""
//	for id, val := range vals {
//		i := id % 2
//		if i == 0 {
//			val_key = val
//		} else if i == 1 {
//			val_map[val_key] = val
//		}
//	}
//	return val_map, val_size, nil
//}

func ParseHashValue(fields, vals []string) (map[string]string, error) {
	val_map := make(map[string]string)
	for i, v := range fields {
		val_map[v] = vals[i]
		i++
	}
	return val_map, nil
}

func ParseZSetValue(val string) (redis.Z, error) {
	zset_val := redis.Z{}

	vals := strings.Split(val, " ")
	if len(vals) != 2 {
		return zset_val, fmt.Errorf("Values len error.")
	}
	score, err := strconv.ParseFloat(vals[0], 64)
	if err != nil {
		return zset_val, fmt.Errorf("Score Error:", err.Error())
	}

	zset_val.Score = score
	zset_val.Member = vals[1]

	return zset_val, nil
}
