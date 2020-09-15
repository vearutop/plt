# Pocket load tester

<img src="https://vignette.wikia.nocookie.net/looneytunes/images/4/46/Plucky_Anvil_2.gif/revision/latest/scale-to-width-down/150?cb=20190522080043" align="right" width="150" height="115" />

Scale curl requests, cousin of `ab`, `siege`, [`hey`](https://github.com/rakyll/hey).

![Code lines](https://sloc.xyz/github/vearutop/plt/?category=code)
![Comments](https://sloc.xyz/github/vearutop/plt/?category=comments)

![plt](https://user-images.githubusercontent.com/1381436/73143999-dd4d5800-40a0-11ea-9308-8e02773ec2d6.gif)

## Install

```
go get -u github.com/vearutop/plt@latest
```

Or 

```
GO111MODULE=on go get github.com/vearutop/plt@latest
```

Or download binary from [releases](https://github.com/vearutop/plt/releases).

## Usage

```
usage: plt [<flags>] <command> [<args> ...]

Pocket load tester pushes to the limit

Flags:
  --help            Show context-sensitive help (also try --help-long and --help-man).
  --number=1000     Number of requests to run, 0 is infinite.
  --concurrency=50  Number of requests to run concurrently.
  --rate-limit=0    Rate limit, in requests per second, 0 disables limit (default).
  --duration=1m     Max duration of load testing, 0 is infinite.
  --slow=1s         Min duration of slow response.
  --live-ui         Show live ui with statistics.

Commands:
  help [<command>...]
    Show help.

  curl [<flags>] [<url>]
    Repetitive HTTP transfer
```

You can "copy as cURL" in your browser and then prepend that with `plt` to throw 1000 of such requests. 

For even better performance you can use `plt curl --fast` that will employ awesome [fasthttp](https://github.com/valyala/fasthttp)
as transport. This mode lacks detailed breakdown of latencies, but can push request rate to the limit.

If the server is wrapped with Envoy proxy, upstream latency distribution will be collected from the values of [`X-Envoy-Upstream-Service-Time`](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#x-envoy-upstream-service-time) response header.

## Example

```bash
plt --live-ui --duration=1m --rate-limit=60 curl -X GET "https://acme-dummy-service.staging-k8s.acme.io/" -H  "accept: application/json"
```

```
Requests per second: 60.79
Total requests: 3650
Time spent: 1m0.041s

Request latency percentiles:
99%: 101.00ms
95%: 45.89ms
90%: 42.58ms
50%: 41.44ms

Request latency distribution in ms:
[   min    max]  cnt total% (3650 events)
[ 39.62  39.62]    1  0.03%
[ 39.67  39.67]    1  0.03%
[ 39.76  39.91]   12  0.33%
[ 39.92  40.46]  217  5.95% .....
[ 40.46  41.71] 2206 60.44% ............................................................
[ 41.71  47.29] 1058 28.99% ............................
[ 47.36  55.04]   74  2.03% ..
[ 55.28  74.25]   18  0.49%
[ 74.40 161.25]   62  1.70% .
[187.05 187.05]    1  0.03%

Requests with latency more than 1s: 0

Envoy upstream latency percentiles:
99%: 12ms
95%: 5ms
90%: 3ms
50%: 2ms

Envoy upstream latency distribution in ms:
[   min    max]  cnt total% (3650 events)
[  1.00   1.00]  474 12.99% ............
[  2.00   2.00] 2157 59.10% ...........................................................
[  3.00   4.00]  809 22.16% ......................
[  5.00   6.00]   69  1.89% .
[  7.00  10.00]   86  2.36% ..
[ 11.00  14.00]   35  0.96%
[ 15.00  23.00]    9  0.25%
[ 28.00  40.00]    5  0.14%
[ 49.00  81.00]    3  0.08%
[ 98.00 148.00]    3  0.08%

DNS latency distribution in ms:
[  min   max] cnt total% (50 events)
[ 9.88  9.88]  1  2.00% ..
[10.13 10.15]  3  6.00% ......
[10.18 10.22]  4  8.00% ........
[10.25 10.31]  5 10.00% ..........
[10.32 10.36]  4  8.00% ........
[10.37 10.41] 12 24.00% ........................
[10.42 10.48]  7 14.00% ..............
[10.49 10.56]  6 12.00% ............
[10.59 10.71]  7 14.00% ..............
[10.76 10.76]  1  2.00% ..

Connection latency distribution in ms:
[  min   max] cnt total% (50 events)
[36.36 36.36]  1  2.00% ..
[36.38 36.39]  6 12.00% ............
[36.39 36.41]  8 16.00% ................
[36.42 36.43]  6 12.00% ............
[36.44 36.48]  5 10.00% ..........
[36.50 36.54]  2  4.00% ....
[36.58 36.66]  2  4.00% ....
[36.70 36.86] 12 24.00% ........................
[36.86 37.09]  4  8.00% ........
[37.11 37.48]  4  8.00% ........

Responses by status code
[200] 3650

Bytes read 1667316
Bytes written 643177
[200]
Welcome to acme-dummy-service. Please read API <a href="/docs/">documentation</a>.
```

### Fast mode

```
plt --concurrency 100 --number 200000 curl --fast -X GET "http://localhost:8011/v0/tasks/1" -H  "accept: application/json"
Host resolved: 127.0.0.1
Requests per second: 145232.05
Total requests: 200000
Request latency distribution in ms:
[  min   max]    cnt total% (200000 events)
[ 0.06  0.06]      1  0.00%
[ 0.06  0.06]      7  0.00%
[ 0.06  0.08]    716  0.36%
[ 0.08  0.10]   5050  2.52% ..
[ 0.10  0.14]  19661  9.83% .........
[ 0.14  0.48] 109190 54.59% ......................................................
[ 0.48  1.07]  46129 23.06% .......................
[ 1.07  2.91]   9715  4.86% ....
[ 2.91 16.01]   9516  4.76% ....
[16.11 23.96]     15  0.01%

Request latency percentiles:
99%: 7.212116ms
95%: 2.914069ms
90%: 1.063058ms
50%: 0.336996ms

Requests with latency more than 1s: 0

Responses by status code
[200] 200000

Bytes read 83600000
Bytes written 18600000
[200]
{"id":1,"goal":"enjoy!","deadline":"2020-05-24T21:00:54.998Z","createdAt":"2020-05-24T23:00:56.656017059+02:00"}
```
