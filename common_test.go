package main

import (
	"fmt"
	//	"fmt"
	//	"strings"
	"encoding/json"
	"testing"

	"gopkg.in/redis.v4"
)

func Test_ErrorNil(t *testing.T) {
	ErrorNil(nil, "a")
	ErrorNil(nil, []string{"a", "b", "c"})
}

func Test_ParseHashValue(t *testing.T) {
	val_map, _, err := ParseHashValue("key0 val0 key1 val1")
	if err != nil {
		t.Errorf("ParseHash Error:%v", err.Error())
	}
	fmt.Println(val_map)
}

func Test_ParseZsetValue(t *testing.T) {
	val_map, err := ParseZSetValue("0 val")
	if err != nil {
		t.Errorf("ParseZset Error:%v", err.Error())
	}
	fmt.Println(val_map)
}

func Test_All(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "192.168.200.135:6379",
		//Password: "Test,123", // no password set
		DB: 0, // use default DB
	})

	_, errclent := client.Ping().Result()
	if errclent != nil {
		fmt.Println("error", errclent.Error())
	} else {
		fmt.Println("success")
	}
	val, err := client.HGetAll("user:1").Result()
	if err == redis.Nil {
		fmt.Println("NIL")
	} else if err != nil {
		fmt.Println("ERR:" + err.Error())
	} else {
		fmt.Println(val)
	}
	b, _ := json.Marshal(val)
	fmt.Println(string(b))

	val1, err := client.HGet("user:1", "password").Result()
	if err == redis.Nil {
		fmt.Println("NIL")
	} else if err != nil {
		fmt.Println("ERR:" + err.Error())
	} else {
		fmt.Println(val1)
	}

	vals, err := client.SMembers("w3cschool.cc").Result()
	if err == redis.Nil {
		fmt.Println("NIL")
	} else if err != nil {
		fmt.Println("ERR:" + err.Error())
	} else {
		fmt.Println(val)
	}

	vals_str := "["
	for _, s := range vals {
		vals_str += `"` + s + `",`
	}
	vals_str = vals_str[:len(vals_str)-1] + `]`
	fmt.Println(vals_str)
	ret := `{"_type":"0","val":` + vals_str + `}`

	fmt.Println(string(ret))

	vals11 := []string{"username"}
	vals1, err := client.HMGet("user:1", vals11...).Result()
	if err == redis.Nil {
		fmt.Println("NIL")
	} else if err != nil {
		fmt.Println("ERR:" + err.Error())
	}
	if len(vals) == 0 {
		ret = `{"_type":"0","val":[]}`
		fmt.Println(ret)
		return
	}
	vals_str1 := "["
	for _, s := range vals1 {
		vals_str1 += `"` + s.(string) + `",`
	}
	vals_str1 = vals_str1[:len(vals_str1)-1] + `]`
	ret = `{"_type":"0","val":` + vals_str1 + `}`
	fmt.Println(ret)

}
