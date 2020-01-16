# Pocket load tester

![Code lines](https://sloc.xyz/github/vearutop/plt/?category=code)
![Comments](https://sloc.xyz/github/vearutop/plt/?category=comments)

This tool scales curl requests, from the same family as `ab`, `siege`, [`hey`](https://github.com/rakyll/hey).

## Install

```
go get -u github.com/vearutop/plt
```

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

## Example

```bash
plt --live-ui --duration=1m --number=0 --rate-limit=60 curl -X GET "https://acme-dummy-service.staging-k8s.acme.io/" -H  "accept: application/json"
```

```
Requests per second: 60.80
Total requests: 3650
Request latency distribution in ms:
[   min    max]  cnt total% (3650 events)
[ 32.17  32.17]    1  0.03%
[ 32.19  32.26]    4  0.11%
[ 32.30  32.62]   20  0.55%
[ 32.64  33.52]  314  8.60% ........
[ 33.52  34.84] 1808 49.53% .................................................
[ 34.84  37.31]  445 12.19% ............
[ 37.31  46.23]  304  8.33% ........
[ 46.24  67.19]  151  4.14% ....
[ 67.44 121.88]  240  6.58% ......
[121.90 319.30]  363  9.95% .........

Requests with latency more than 1s: 0
DNS latency distribution in ms:
[ min  max] cnt total% (61 events)
[0.03 0.03]  1  1.64% .
[0.18 0.18]  1  1.64% .
[0.48 0.48]  1  1.64% .
[0.66 0.80]  3  4.92% ....
[0.87 1.10]  6  9.84% .........
[1.61 1.89] 10 16.39% ................
[1.98 2.50] 17 27.87% ...........................
[2.62 2.83]  8 13.11% .............
[2.97 3.43] 12 19.67% ...................
[3.89 3.95]  2  3.28% ...

TLS handshake latency distribution in ms:
[   min    max] cnt total% (61 events)
[ 78.60  78.60]  1  1.64% .
[ 78.63  78.63]  1  1.64% .
[ 78.66  78.66]  1  1.64% .
[ 91.66  91.66]  1  1.64% .
[ 92.93  92.94]  2  3.28% ...
[ 94.22  94.56]  3  4.92% ....
[ 95.14  95.57]  2  3.28% ...
[203.56 210.54] 48 78.69% ..............................................................................
[241.46 241.46]  1  1.64% .
[415.45 415.45]  1  1.64% .

Connection latency distribution in ms:
[  min   max] cnt total% (61 events)
[31.86 31.86]  1  1.64% .
[31.89 31.89]  1  1.64% .
[32.17 32.22]  4  6.56% ......
[32.26 32.35]  5  8.20% ........
[32.72 32.72]  1  1.64% .
[33.19 33.50]  7 11.48% ...........
[33.89 34.13]  7 11.48% ...........
[34.94 35.88] 14 22.95% ......................
[37.01 38.08] 19 31.15% ...............................
[40.07 40.33]  2  3.28% ...

Responses by status code
[200] 3650

Bytes read 1667316
Bytes written 643177
[200]
Welcome to acme-dummy-service. Please read API <a href="/docs/">documentation</a>.
```