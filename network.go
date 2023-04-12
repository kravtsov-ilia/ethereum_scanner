package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

type getBlockReq struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	Id      string        `json:"id"`
}

type getLastBlockResp struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      string `json:"id"`
	Result  string `json:"result"`
}

type transaction struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"`
}

type getBlockByNumbResp struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      string `json:"id"`
	Result  struct {
		Transactions []transaction `json:"transactions"`
	} `json:"result"`
}

func doGetBlockReq(client *http.Client, conf *Config, data getBlockReq) ([]byte, error) {
	jsonData, _ := json.Marshal(data)
	req, err := http.NewRequest(http.MethodPost, conf.ApiUrl, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", conf.ApiKey)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}
