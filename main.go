package main

import (
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"
)

const LastBlocksCount = 100

type Config struct {
	ApiKey string
	ApiUrl string
}

type walletsState struct {
	sync.Mutex
	wallets map[string]*big.Int
}

func (w *walletsState) changeState(walletAddr string, value *big.Int) {
	w.Lock()
	defer w.Unlock()
	if balance, ok := w.wallets[walletAddr]; ok {
		w.wallets[walletAddr].Add(balance, value)
	} else {
		w.wallets[walletAddr] = value
	}

}

type txDiffData struct {
	WalletAddress string
	Value         *big.Int
}

func hexToDec(hexVal string) (*big.Int, error) {
	decValue := big.NewInt(0)
	_, isOk := decValue.SetString(hexVal[2:], 16)
	if !isOk {
		return nil, fmt.Errorf("can not convert hex - %s", hexVal)
	}
	return decValue, nil
}

func parseBlockTransactions(
	blockNumber int64,
	txChannel chan<- *txDiffData,
	conf *Config,
) {
	client := http.Client{
		Timeout: 30 * time.Second,
	}

	params := make([]interface{}, 0, 2)
	params = append(params, fmt.Sprintf("0x%x", blockNumber), true)
	reqData := getBlockReq{
		Jsonrpc: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  params,
		Id:      "getblock.io",
	}
	var (
		respBody []byte
		err      error
	)
	const requestsMaxRetries = 10
	requestsCounter := 0
	for {
		respBody, err = doGetBlockReq(
			&client,
			conf,
			reqData)
		requestsCounter += 1
		if err != nil {
			panic(fmt.Sprintf("err: %s\n", err))
		}
		if len(respBody) > 0 {
			break
		} else if requestsCounter >= requestsMaxRetries {
			panic(fmt.Sprintf("requests limit has been exceeded, block number - %d", blockNumber))
		}
	}

	var blockData getBlockByNumbResp
	err = json.Unmarshal(respBody, &blockData)
	if err != nil {
		panic(fmt.Sprintf("can not parse tx for block: %d\n%s", blockNumber, err))
	}
	for _, tx := range blockData.Result.Transactions {
		if tx.Value == "0x0" {
			continue
		}
		decValue, err := hexToDec(tx.Value)
		if err != nil {
			panic(err)
		}
		txChannel <- &txDiffData{
			WalletAddress: tx.From,
			Value:         big.NewInt(0).Mul(decValue, big.NewInt(-1)),
		}
		txChannel <- &txDiffData{
			WalletAddress: tx.To,
			Value:         decValue,
		}
	}
}

func getLastBlockNumber(conf *Config) (int64, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	reqData := getBlockReq{
		Jsonrpc: "2.0",
		Method:  "eth_blockNumber",
		Params:  nil,
		Id:      "getblock.io",
	}
	respBody, err := doGetBlockReq(
		client,
		conf,
		reqData)
	if err != nil {
		return 0, err
	}
	var respData getLastBlockResp
	err = json.Unmarshal(respBody, &respData)
	if err != nil {
		return 0, err
	}
	lastBlockNumber, err := hexToDec(respData.Result)
	if err != nil {
		return 0, fmt.Errorf("can not convert hex to decimal: %s", respData.Result)
	}
	return lastBlockNumber.Int64(), nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		panic(fmt.Sprintf("error while loading .env file %s", err))
	}
	conf := Config{
		ApiKey: os.Getenv("API_KEY"),
		ApiUrl: os.Getenv("API_ENDPOINT"),
	}

	lastBlockNumber, err := getLastBlockNumber(&conf)
	if err != nil {
		panic(err)
	}

	var (
		blockNumb      int64
		wgTransactions sync.WaitGroup
		wgBalance      sync.WaitGroup
	)
	txChan := make(chan *txDiffData, 50*2*LastBlocksCount)

	// run transaction scanners
	wgTransactions.Add(LastBlocksCount)
	for i := int64(0); i < LastBlocksCount; i++ {
		blockNumb = lastBlockNumber - i
		go func(n int64, w *sync.WaitGroup, c chan<- *txDiffData) {
			defer w.Done()
			parseBlockTransactions(n, txChan, &conf)
		}(blockNumb, &wgTransactions, txChan)
	}

	walletsBalances := walletsState{
		wallets: make(map[string]*big.Int),
	}

	// start listen balances diff
	wgBalance.Add(1)
	go func(c <-chan *txDiffData, w *sync.WaitGroup) {
		for txData := range c {
			walletsBalances.changeState(txData.WalletAddress, txData.Value)
		}
		w.Done()
	}(txChan, &wgBalance)

	wgTransactions.Wait()
	close(txChan)
	wgBalance.Wait()

	// find max value
	maxBalanceDiff := big.NewInt(0)
	var walletMaxBalanceDiff string
	for k, v := range walletsBalances.wallets {
		if maxBalanceDiff.Abs(maxBalanceDiff).Cmp(v) < 0 {
			maxBalanceDiff = v.Abs(v)
			walletMaxBalanceDiff = k
		}
	}
	fmt.Printf("For the last %d blocks wallet with max balance diff - %s; diff value - %d wei\n",
		LastBlocksCount,
		walletMaxBalanceDiff,
		maxBalanceDiff,
	)
}
