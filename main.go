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
	concurrency int64
	totalNumber int64
	duration    int64
	url         string
	filePath    string
	httpMethod  string
	postStr     string
	timeout     int64
}

type HttpRequest struct {
	url     string
	postStr string
	timeout time.Duration
	method  string
}

type Result struct {
	startTime  time.Time
	endTime    time.Time
	httpResult []*HttpResult
}

type HttpResult struct {
	latency time.Duration
	code    int
	bytes   int64
}

func main() {
	cmd, err := parseCmd()
	if err != nil {
		return
	}

	result := work(cmd)

	printResult(result)
}

func parseCmd() (cmd Cmd, err error) {

	flag.Int64Var(&cmd.concurrency, "c", 0, "并发数")
	flag.Int64Var(&cmd.totalNumber, "n", 0, "请求总次数")
	flag.Int64Var(&cmd.duration, "a", 0, "请求总时长，单位：s")
	flag.Int64Var(&cmd.timeout, "t", 1, "默认超时时间1ms")
	flag.StringVar(&cmd.url, "u", "", "请求的url")
	flag.StringVar(&cmd.filePath, "f", "", "文件路径")
	flag.StringVar(&cmd.httpMethod, "m", "get", "HTTP method，默认：get")
	flag.StringVar(&cmd.postStr, "d", "", "post参数")

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

	if cmd.totalNumber == 0 && cmd.duration == 0 {
		fmt.Println("-n 和 -a 必须填一项")
		usage()

		err = errors.New("parseCmd error")
		return
	}

	if cmd.httpMethod != "get" && cmd.httpMethod != "post" {
		fmt.Println("-m 只能是get 或者 post")
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
  -a  请求持续时间，单位：秒，优先使用-n参数
  -m  HTTP method，默认：get
  -u  请求的url
  -d  post时的请求参数
  -f  从文件中获取可变参数信息
        get方式时，请求的地址是url和文件里面的行拼接而成
        post方式，文件里面是post的参数
  -t  超时时间，默认：1s
  -h  帮助`)
}

/**
 * 压测过程
 */
func work(cmd Cmd) (result Result) {
	httpRequestList := produceHttpRequest(cmd)

	// 如果是总次数发压，拼接好cmd.totalNumber总数的请求，
	// 如果是按持续时间发压，设置好stop标志位，由定时器修改
	stop := false
	if cmd.totalNumber != 0 {
		if cmd.totalNumber > int64(len(httpRequestList)) {
			index := 0
			for i := int64(0); i < cmd.totalNumber-int64(len(httpRequestList)); i++ {
				if index >= len(httpRequestList) {
					index = 0
				}
				httpRequestList = append(httpRequestList, httpRequestList[index])
			}
		}
	} else {
		go func() {
			duration := 0
			ticker := time.NewTicker(time.Second)
			for range ticker.C {
				duration++
				if int64(duration) >= cmd.duration {
					stop = true
					break
				}
			}
		}()
	}

	resultList := make([][]*HttpResult, cmd.concurrency)
	result.startTime = time.Now()
	var wg sync.WaitGroup
	for i := int64(0); i < cmd.concurrency; i++ {
		wg.Add(1)
		go func(_i int64) {
			defer wg.Done()

			resultList[_i] = make([]*HttpResult, 0)
			// 同时支持总次数和总时间
			if cmd.totalNumber != 0 {
				for _, _httpRequest := range httpRequestList {
					resultList[_i] = append(resultList[_i], sendHttp(_httpRequest))
				}
			} else {
				// 每十次判断一次结束状态
				start := 0
				end := 0
				for ; ; {
					if stop == true {
						break
					}

					start = start + 10
					end = start + 10
					if start >= len(httpRequestList) {
						start = 0
						end = start + 10
					}
					if end >= len(httpRequestList) {
						end = len(httpRequestList)
					}
					for _, _httpRequest := range httpRequestList[start:end] {
						resultList[_i] = append(resultList[_i], sendHttp(_httpRequest))
					}
				}
			}
		}(i)
	}
	wg.Wait()
	result.endTime = time.Now()

	httpResultTmp := make([]*HttpResult, 0)
	for _, result := range resultList {
		for _, _result := range result {
			httpResultTmp = append(httpResultTmp, _result)
		}
	}
	result.httpResult = httpResultTmp
	return result
}

/**
 * 构造HttpRequest
 */
func produceHttpRequest(cmd Cmd) (httpRequestList []*HttpRequest) {
	method := cmd.httpMethod
	timeout := time.Second * time.Duration(cmd.timeout)

	httpRequestList = make([]*HttpRequest, 0)
	totalNumber := cmd.totalNumber
	if totalNumber == 0 {
		totalNumber = 10
	}

	// 读取文件
	fileLines := make([]string, 0)
	if cmd.filePath != "" {
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
	}

	// get方式下，文件里面读出来的可变参数信息拼接到url后面
	if method == "get" {
		if len(fileLines) == 0 {
			for i := int64(0); i < totalNumber; i++ {
				httpRequest := new(HttpRequest)
				httpRequest.url = cmd.url
				httpRequest.method = method
				httpRequest.timeout = timeout
				httpRequestList = append(httpRequestList, httpRequest)
			}
		} else {
			fileLen := len(fileLines)
			for i := 0; i < fileLen; i++ {
				httpRequest := new(HttpRequest)
				httpRequest.url = cmd.url + fileLines[i]
				httpRequest.method = method
				httpRequest.timeout = timeout
				httpRequestList = append(httpRequestList, httpRequest)
			}
		}
	} else {
		// post方式下，文件里面读出来的可变参数信息放入data字段
		postStr := ""
		if cmd.postStr != "" {
			postStr = cmd.postStr + "&"
		}

		if len(fileLines) == 0 {
			for i := int64(0); i < totalNumber; i++ {
				httpRequest := new(HttpRequest)
				httpRequest.url = cmd.url
				httpRequest.method = method
				httpRequest.timeout = timeout
				httpRequest.postStr = postStr
				httpRequestList = append(httpRequestList, httpRequest)
			}
		} else {
			fileLen := len(fileLines)
			for i := 0; i < fileLen; i++ {
				httpRequest := new(HttpRequest)
				httpRequest.url = cmd.url
				httpRequest.postStr = postStr + fileLines[i]
				httpRequest.method = method
				httpRequest.timeout = timeout
				httpRequestList = append(httpRequestList, httpRequest)
			}
		}
	}

	return httpRequestList
}

/**
 * 单个http请求
 */
func sendHttp(httpRequest *HttpRequest) (httpResult *HttpResult) {

	httpResult = new(HttpResult)

	if httpRequest.method == "get" {
		startTime := time.Now()
		res, body, err := gorequest.New().Timeout(httpRequest.timeout).Get(httpRequest.url).EndBytes()
		if err != nil {
			httpResult.code = http.StatusBadRequest
			httpResult.latency = time.Since(startTime)
		} else {
			httpResult.code = res.StatusCode
			httpResult.latency = time.Since(startTime)
			httpResult.bytes = int64(len(body))
		}

		return
	} else {
		startTime := time.Now()
		res, body, err := gorequest.New().Timeout(httpRequest.timeout).Type(gorequest.TypeUrlencoded).
			Post(httpRequest.url).Send(httpRequest.postStr).EndBytes()
		if err != nil {
			httpResult.code = http.StatusBadRequest
			httpResult.latency = time.Since(startTime)
		} else {
			httpResult.code = res.StatusCode
			httpResult.latency = time.Since(startTime)
			httpResult.bytes = int64(len(body))
		}

		return
	}
}

/**
 * 打印结果
 */
func printResult(result Result) {
	totalRequestTime := float64(result.endTime.Sub(result.startTime)) / float64(time.Second)
	totalRequest := len(result.httpResult)

	successRequest := 0
	failedRequest := 0
	statusMap := make(map[int]int64)
	latencyList := make([]float64, 0)
	successTotalTime := float64(0)
	successTotalBytes := int64(0)
	for _, _httpResult := range result.httpResult {
		if _httpResult.code == http.StatusOK {
			successRequest++
			successTotalTime = successTotalTime + float64(_httpResult.latency/time.Millisecond)
			successTotalBytes = successTotalBytes + _httpResult.bytes
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
	fmt.Printf("	总请求数:	%d\n", totalRequest)
	fmt.Printf("	总测试时长:	%f(s)\n", totalRequestTime)
	fmt.Printf("	QPS:	%f\n", float64(successRequest)/totalRequestTime)
	fmt.Printf("	获取数据速率:	%f(KB/s)\n", float64(successTotalBytes)/1024/totalRequestTime)
	fmt.Printf("	访问成功接口平均耗时:	%f(ms)\n", successTotalTime/float64(successRequest))

	fmt.Println("\n接口请求耗时分布(ms):")
	fmt.Printf("	50.00%%:	%f\n", stepLatency[0.5])
	fmt.Printf("	90.00%%:	%f\n", stepLatency[0.9])
	fmt.Printf("	95.00%%:	%f\n", stepLatency[0.95])
	fmt.Printf("	99.00%%:	%f\n", stepLatency[0.99])
	fmt.Printf("	99.99%%:	%f\n", stepLatency[0.9999])
}
