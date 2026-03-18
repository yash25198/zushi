package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	_ "embed"
)

//go:embed index.html
var indexHTML []byte

var (
	rpcURL  string
	rpcUser string
	rpcPass string
)

func main() {
	rpcHost := env("ZCASH_RPC_HOST", "zcashd")
	rpcPort := env("ZCASH_RPC_PORT", "18232")
	rpcUser = env("ZCASH_RPC_USER", "zcashrpc")
	rpcPass = env("ZCASH_RPC_PASSWORD", "zcashpass")
	listen := env("LISTEN_ADDR", "0.0.0.0:8080")

	rpcURL = fmt.Sprintf("http://%s:%s", rpcHost, rpcPort)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	http.HandleFunc("/api/rpc", handleRPC)
	http.HandleFunc("/api/blocks", handleBlocks)
	http.HandleFunc("/api/block/", handleBlock)
	http.HandleFunc("/api/tx/", handleTx)
	http.HandleFunc("/api/address/", handleAddress)
	http.HandleFunc("/api/mempool", handleMempool)
	http.HandleFunc("/api/info", handleInfo)

	log.Printf("zushi explorer listening on %s", listen)
	if err := http.ListenAndServe(listen, nil); err != nil {
		log.Fatal(err)
	}
}

func handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Method string        `json:"method"`
		Params []interface{} `json:"params"`
	}
	json.Unmarshal(body, &req)
	result, err := rpcCall(req.Method, req.Params)
	if err != nil {
		jsonErr(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result))
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	result, err := rpcCall("getblockchaininfo", nil)
	if err != nil {
		jsonErr(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result))
}

func handleBlocks(w http.ResponseWriter, r *http.Request) {
	// Get best block height
	countRaw, err := rpcCall("getblockcount", nil)
	if err != nil {
		jsonErr(w, err.Error(), 500)
		return
	}
	var height int
	json.Unmarshal([]byte(countRaw), &height)

	limit := 20
	blocks := make([]json.RawMessage, 0, limit)
	for i := height; i >= 0 && len(blocks) < limit; i-- {
		hashRaw, err := rpcCall("getblockhash", []interface{}{i})
		if err != nil {
			break
		}
		var hash string
		json.Unmarshal([]byte(hashRaw), &hash)

		blockRaw, err := rpcCall("getblock", []interface{}{hash, 1})
		if err != nil {
			break
		}
		blocks = append(blocks, json.RawMessage(blockRaw))
	}

	w.Header().Set("Content-Type", "application/json")
	out, _ := json.Marshal(blocks)
	w.Write(out)
}

func handleBlock(w http.ResponseWriter, r *http.Request) {
	hashOrHeight := strings.TrimPrefix(r.URL.Path, "/api/block/")
	if hashOrHeight == "" {
		jsonErr(w, "missing block hash or height", 400)
		return
	}

	hash := hashOrHeight
	// If it looks like a number, resolve to hash first
	if len(hashOrHeight) < 64 {
		var height int
		fmt.Sscanf(hashOrHeight, "%d", &height)
		hashRaw, err := rpcCall("getblockhash", []interface{}{height})
		if err != nil {
			jsonErr(w, err.Error(), 404)
			return
		}
		json.Unmarshal([]byte(hashRaw), &hash)
	}

	result, err := rpcCall("getblock", []interface{}{hash, 2})
	if err != nil {
		jsonErr(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result))
}

func handleTx(w http.ResponseWriter, r *http.Request) {
	txid := strings.TrimPrefix(r.URL.Path, "/api/tx/")
	if txid == "" {
		jsonErr(w, "missing txid", 400)
		return
	}

	result, err := rpcCall("getrawtransaction", []interface{}{txid, 1})
	if err != nil {
		jsonErr(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result))
}

func handleAddress(w http.ResponseWriter, r *http.Request) {
	addr := strings.TrimPrefix(r.URL.Path, "/api/address/")
	if addr == "" {
		jsonErr(w, "missing address", 400)
		return
	}

	// Try z_getbalance for both transparent and shielded addresses
	balRaw, err := rpcCall("z_getbalance", []interface{}{addr})
	if err != nil {
		jsonErr(w, err.Error(), 404)
		return
	}

	// Get received by address (transparent only)
	receivedRaw, _ := rpcCall("getreceivedbyaddress", []interface{}{addr, 0})

	resp := map[string]json.RawMessage{
		"address": json.RawMessage(fmt.Sprintf("%q", addr)),
		"balance": json.RawMessage(balRaw),
	}
	if receivedRaw != "" {
		resp["totalReceived"] = json.RawMessage(receivedRaw)
	}

	w.Header().Set("Content-Type", "application/json")
	out, _ := json.Marshal(resp)
	w.Write(out)
}

func handleMempool(w http.ResponseWriter, r *http.Request) {
	result, err := rpcCall("getrawmempool", nil)
	if err != nil {
		jsonErr(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result))
}

func rpcCall(method string, params []interface{}) (string, error) {
	if params == nil {
		params = []interface{}{}
	}
	payload := map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      "zushi-explorer",
		"method":  method,
		"params":  params,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", rpcURL, bytes.NewReader(body))
	req.SetBasicAuth(rpcUser, rpcPass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("rpc: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return "", fmt.Errorf("rpc parse: %w", err)
	}
	if rpcResp.Error != nil {
		return "", fmt.Errorf("%s", rpcResp.Error.Message)
	}
	return string(rpcResp.Result), nil
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
