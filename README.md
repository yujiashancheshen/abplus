## 使用说明
```
选项：
  -c  并发数
  -n  请求总次数
  -m  HTTP method，默认：get
  -u  请求的url
  -f  从文件中获取请求信息
  -h  帮助
```

## 结果
```
结果:
  成功请求数:		29000
  失败请求数:		100
  总测试时长(s):		10
  QPS:		        2900
  平均响应时间(ms):       200

接口请求耗时分布(ms):
    50.00%:		10
    60.00%:		10
    70.00%:		20
    80.00%:		30
    90.00%:		40
    99.00%:		50
    99.99%:		60
```