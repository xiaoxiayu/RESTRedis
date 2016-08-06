package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Get node ip from metadata -> labels -> kubernetes.io/hostname
func GetNodes(rest_url, label string) ([]string, error) {
	node_ips := []string{}
	tem_lable := strings.Split(label, ":")
	if len(tem_lable) != 2 {
		return nil, fmt.Errorf("nodelabel set error.")
	}
	label_key := tem_lable[0]
	label_val := tem_lable[1]

	res, err := HTTPGet(rest_url)
	if err != nil {
		return nil, err
	}

	var k8data interface{}
	err = json.Unmarshal(res, &k8data)
	if err != nil {
		return nil, err
	}
	m := k8data.(map[string]interface{})

	for _, v := range m {
		switch vv := v.(type) {
		case []interface{}:
			for _, v1 := range vv {
				vvv := v1.(map[string]interface{})
				metadata_if := vvv["metadata"]
				metadata := metadata_if.(map[string]interface{})

				labels_if := metadata["labels"]
				labels := labels_if.(map[string]interface{})
				if labels[label_key] != label_val {
					continue
				}
				node_ips = append(node_ips, labels["kubernetes.io/hostname"].(string))
			}
		}
	}
	if len(node_ips) == 0 {
		return node_ips, fmt.Errorf("Nodes is empty.")
	}
	return node_ips, nil
}
