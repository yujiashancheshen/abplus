package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"github.com/parnurzeal/gorequest"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

type Cmd struct {
	concurrency uint64
	totalNumber uint64
	url         string
	filePath    string
	httpMethod  string
}

type Result struct {
	startTime  time.Time
	endTime    time.Time
	httpResult []*HttpResult
}

type HttpResult struct {
	latency time.Duration
	code    int
}

func main() {
	cmd, err := parseCmd()
	if err != nil {
		return
	}

	result := start(cmd)

	printResult(result)
}

func parseCmd() (cmd Cmd, err error) {

	flag.Uint64Var(&cmd.concurrency, "c", 0, "并发数")
	flag.Uint64Var(&cmd.totalNumber, "n", 0, "请求总次数")
	flag.StringVar(&cmd.url, "u", "", "请求的url")
	flag.StringVar(&cmd.filePath, "f", "", "文件路径")
	flag.StringVar(&cmd.httpMethod, "m", "GET", "HTTP method，默认：get")

	flag.Parse()
	if cmd.url == "" && cmd.filePath == "" {
		fmt.Println("-u 和 -f 不能同时为空")
		usage()

		err = errors.New("parseCmd error")
		return
	}

	if cmd.concurrency == 0 {
		fmt.Println("-c 必须填")
		usage()

		err = errors.New("parseCmd error")
		return
	}

	if cmd.totalNumber == 0 {
		fmt.Println("-n 必须填")
		usage()

		err = errors.New("parseCmd error")
		return
	}

	fmt.Println("压测开始，请耐心等待.....")
	return
}

func usage() {
	fmt.Println(`选项：
  -c  并发数
  -n  请求总次数
  -m  HTTP method，默认：get
  -u  请求的url
  -f  从文件中获取请求信息
  -h  帮助`)
}

func start(cmd Cmd) (result Result) {
	result.httpResult = make([]*HttpResult, 0, cmd.totalNumber)
	urlChan := make(chan string, cmd.totalNumber)

	if cmd.filePath != "" {
		fileLines := make([]string, 0)

		f, _ := os.Open(cmd.filePath)
		r := bufio.NewReader(f)
		for {
			line, _, err := r.ReadLine()
			if err != nil {
				break
			}
			fileLines = append(fileLines, string(line))
		}
		f.Close()

		index := 0
		fileLen := len(fileLines)
		for i := uint64(0); i < cmd.totalNumber; i++ {
			if index >= fileLen {
				index = 0
			}
			urlChan <- cmd.url + fileLines[index]
			index++
		}
	} else {
		for i := uint64(0); i < cmd.totalNumber; i++ {
			urlChan <- cmd.url
		}
	}
	close(urlChan)

	resultChan := make(chan *HttpResult, cmd.totalNumber)

	result.startTime = time.Now()
	var wg sync.WaitGroup
	for i := uint64(0); i < cmd.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for url := range urlChan {
				_startTime := time.Now()
				res, _, err := gorequest.New().Get(url).EndBytes()
				httpResult := new(HttpResult)
				if err != nil {
					httpResult.code = http.StatusBadRequest
					httpResult.latency = time.Since(_startTime)
				}
				httpResult.code = res.StatusCode
				httpResult.latency = time.Since(_startTime)

				resultChan <- httpResult
			}
		}()
	}
	wg.Wait()
	result.endTime = time.Now()

	close(resultChan)
	for _result := range resultChan {
		result.httpResult = append(result.httpResult, _result)
	}
	return result
}

func printResult(result Result) {
	totalTime := result.endTime.Sub(result.startTime)
	totalRequest := len(result.httpResult)

	successRequest := 0
	failedRequest := 0

	statusMap := make(map[int]int64)

	latencyList := make([]float64, 0)

	for _, _httpResult := range result.httpResult {
		if _httpResult.code == http.StatusOK {
			successRequest++
		} else {
			failedRequest++
		}

		_, ok := statusMap[_httpResult.code]
		if ok {
			statusMap[_httpResult.code]++
		} else {
			statusMap[_httpResult.code] = 1
		}

		latencyList = append(latencyList, float64(_httpResult.latency/time.Millisecond))
	}

	stepLatency := make(map[float64]float64)
	sort.Float64s(latencyList)
	latencyListLen := len(latencyList)
	steps := []float64{0.5, 0.9, 0.95, 0.99, 0.9999}
	for _, step := range steps {
		stepLatency[step] = latencyList[int(float64(latencyListLen)*step)]
	}

	fmt.Println("\n结果：")
	fmt.Printf("	成功请求数:	%d\n", successRequest)
	fmt.Printf("	失败请求数:	%d\n", failedRequest)
	fmt.Printf("	总测试时长:	%f(s)\n", float64(totalTime)/float64(time.Second))
	fmt.Printf("	总请求数:	%d\n", totalRequest)
	fmt.Printf("	QPS:	%f\n", float64(totalRequest)/(float64(totalTime)/float64(time.Second)))

	fmt.Println("\n接口请求耗时分布(ms):")
	fmt.Printf("	50.00%%:	%f\n", stepLatency[0.5])
	fmt.Printf("	90.00%%:	%f\n", stepLatency[0.9])
	fmt.Printf("	95.00%%:	%f\n", stepLatency[0.95])
	fmt.Printf("	99.00%%:	%f\n", stepLatency[0.99])
	fmt.Printf("	99.99%%:	%f\n", stepLatency[0.9999])
}
