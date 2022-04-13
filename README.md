# Pocket load tester

<img src="https://vignette.wikia.nocookie.net/looneytunes/images/4/46/Plucky_Anvil_2.gif/revision/latest/scale-to-width-down/150?cb=20190522080043" align="right" width="150" height="115" />

Scale curl requests, cousin of `ab`, `siege`, [`hey`](https://github.com/rakyll/hey).

![Code lines](https://sloc.xyz/github/vearutop/plt/?category=code)
![Comments](https://sloc.xyz/github/vearutop/plt/?category=comments)

## Demo

![Demo](./demo.svg)

## Install

```
go install github.com/vearutop/plt@latest
$(go env GOPATH)/bin/plt --help
```

Or (with `go1.15` or older)

```
go get -u github.com/vearutop/plt
$(go env GOPATH)/bin/plt --help
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

Use `--http2` or `--http3` for HTTP/2 or HTTP/3.

If the server is wrapped with Envoy proxy, upstream latency distribution will be collected from the values of [`X-Envoy-Upstream-Service-Time`](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#x-envoy-upstream-service-time) response header.

In `--live-ui` mode you can control concurrency and rate limits with arrow keys.

## Example

```bash
plt --live-ui --duration=20s --rate-limit=60 curl -X GET "https://demo.phperf.ml/profile"
```

```
plt --live-ui --duration=20s --rate-limit=60 curl -X GET "https://demo.phperf.ml/profile"
Host resolved: 130.61.99.204

Requests per second: 59.96
Total requests: 1201
Time spent: 20.029s

Request latency percentiles:
99%: 101.73ms
95%: 64.88ms
90%: 44.57ms
50%: 28.09ms

Request latency distribution in ms:
[   min    max]  cnt total% (1201 events)
[ 19.03  19.03]    1  0.08%
[ 22.52  23.76]   25  2.08% ..
[ 23.78  25.26]  111  9.24% .........
[ 25.26  26.58]  200 16.65% ................
[ 26.58  29.61]  449 37.39% .....................................
[ 29.61  39.94]  287 23.90% .......................
[ 40.54  40.54]    1  0.08%
[ 41.00  62.45]   60  5.00% ....
[ 62.77 110.91]   57  4.75% ....
[126.64 260.53]   10  0.83%

Requests with latency more than 1s: 0

Bytes read 2.3MB / 23.5MB/s
Bytes written 105.4KB / 57.6KB/s

DNS latency distribution in ms:
[ min  max] cnt total% (16 events)
[0.44 0.44]  1  6.25% ......
[0.49 0.49]  1  6.25% ......
[0.50 0.50]  1  6.25% ......
[0.56 0.56]  1  6.25% ......
[0.61 0.61]  2 12.50% ............
[0.65 0.70]  3 18.75% ..................
[0.77 0.77]  1  6.25% ......
[0.88 0.88]  1  6.25% ......
[1.00 1.03]  3 18.75% ..................
[1.28 1.42]  2 12.50% ............

TLS handshake latency distribution in ms:
[   min    max] cnt total% (16 events)
[ 64.95  64.95]  1  6.25% ......
[ 65.14  65.14]  1  6.25% ......
[ 66.98  66.98]  1  6.25% ......
[ 70.94  71.81]  3 18.75% ..................
[ 74.10  76.13]  2 12.50% ............
[ 80.20  82.91]  2 12.50% ............
[ 90.86  90.86]  1  6.25% ......
[127.14 127.72]  2 12.50% ............
[157.99 161.84]  2 12.50% ............
[179.98 179.98]  1  6.25% ......

Time to first resp byte (TTFB) distribution in ms:
[   min    max]  cnt total% (1201 events)
[ 18.97  18.97]    1  0.08%
[ 22.49  23.77]   27  2.25% ..
[ 23.78  24.96]   81  6.74% ......
[ 24.96  26.56]  236 19.65% ...................
[ 26.57  29.52]  441 36.72% ....................................
[ 29.54  29.54]    1  0.08%
[ 29.56  40.90]  288 23.98% .......................
[ 41.16  62.68]   60  5.00% ....
[ 63.02 110.87]   56  4.66% ....
[126.59 260.27]   10  0.83%

Connection latency distribution in ms:
[  min   max] cnt total% (16 events)
[28.04 28.04]  1  6.25% ......
[28.13 28.13]  1  6.25% ......
[29.68 29.68]  1  6.25% ......
[30.01 30.41]  2 12.50% ............
[31.04 31.12]  2 12.50% ............
[32.22 32.44]  2 12.50% ............
[33.37 34.07]  3 18.75% ..................
[36.04 36.04]  1  6.25% ......
[40.09 40.09]  1  6.25% ......
[43.50 45.20]  2 12.50% ............

Responses by status code
[200] 1201

Response samples (first by status code):
[200]
Connection: keep-alive
Content-Length: 1733
Content-Type: application/json; charset=utf-8
Date: Thu, 08 Apr 2021 15:25:54 GMT
Server: nginx/1.14.0 (Ubuntu)

{"recent":[{"addr":{"id":"606f1f90dcdc3"},"edges":3715,"wt":"190.55ms","cpu":"179.03ms","io":"11.52ms","peakMem":"3.52M"},{"addr":{"id":"606f1faf1c5c3"},"edges":3715,"wt":"270.14ms","cpu":"185.62ms","io":"84.52ms","peakMem":"3.52M"},{"addr":{"id":"606f1fc261e9d"},"edges":575,"wt":"3.4s","cpu":"3.39s","io":"9.71ms","peakMem":"28.03M"},{"addr":{"id":"606f1fcd6f694"},"edges":3715,"wt":"153.58ms","cpu":"143.68ms","io":"9.9ms","peakMem":"3.52M"},{"addr":{"id":"606f1feba911d"},"edges":3715,"wt":"202.18ms","cpu":"191.82ms","io":"10.36ms","peakMem":"3.52M"},{"addr":{"id":"606f20052f7c1"},"edges":471,"wt":"679.08ms","cpu":"669.01ms","io":"10.07ms","peakMem":"6.18M"},{"addr":{"id":"606f2009de4cd"},"edges":3715,"wt":"175.34ms","cpu":"163.14ms","io":"12.2ms","peakMem":"3.52M"},{"addr":{"id":"606f2028185dd"},"edges":3715,"wt":"677.03ms","cpu":"320.84ms","io":"356.19ms","peakMem":"3.52M"},{"addr":{"id":"606f2046c16a2"},"edges":3715,"wt":"313.28ms","cpu":"292.38ms","io":"20.9ms","peakMem":"3.52M"},{"...
```
