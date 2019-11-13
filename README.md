# Pocket load tester

![Code lines](https://sloc.xyz/github/vearutop/plt/?category=code)
![Comments](https://sloc.xyz/github/vearutop/plt/?category=comments)

This tool scales curl requests, from the same family as `ab`, `siege`, [`hey`](https://github.com/rakyll/hey).

## Usage

```
usage: plt [<flags>] <command> [<args> ...]

Pocket load tester pushes to the limit

Flags:
  --help      Show context-sensitive help (also try --help-long and --help-man).
  --num=1000  Number of requests to run, 0 is infinite.
  --cnc=50    Number of requests to run concurrently.
  --rl=0      Rate limit, in requests per second, 0 disables limit (default).
  --dur=1m    Max duration of load testing, 0 is infinite.
  --slow=1s   Min duration of slow response.

Commands:
  help [<command>...]
    Show help.

  curl [<flags>] [<url>]
    Repetitive HTTP transfer
```

You can "copy as cURL" in your browser and then prepend that with `plt` to throw 1000 of such requests. 

## Example

```bash
plt --num 10000 --rl 6000 curl http://127.0.0.1:8000/
```

```
Starting
Requests per second: 5796.01
Total requests: 10000
Request latency distribution in ms:
[  min   max]   cnt total% (10000 events)
[ 0.09  0.09]     1  0.01%
[ 0.09  0.10]     9  0.09%
[ 0.10  0.10]    15  0.15%
[ 0.10  0.13]   409  4.09% ....
[ 0.13  0.37]  6331 63.31% ...............................................................
[ 0.37  0.37]     1  0.01%
[ 0.38  1.94]  1724 17.24% .................
[ 1.94  6.09]   884  8.84% ........
[ 6.10 15.41]   479  4.79% ....
[15.43 48.87]   147  1.47% .

Requests with latency more than 1s: 0
DNS latency distribution in ms:
[ min  max] cnt total% (0 events)

TLS handshake latency distribution in ms:
[ min  max] cnt total% (0 events)

Connection latency distribution in ms:
[  min   max] cnt total% (92 events)
[ 0.18  0.18]  1  1.09% .
[ 1.52  1.52]  1  1.09% .
[ 7.86  8.60]  3  3.26% ...
[ 9.55  9.55]  1  1.09% .
[12.92 12.92]  1  1.09% .
[15.12 15.87]  3  3.26% ...
[17.09 20.45] 25 27.17% ...........................
[20.52 23.01] 22 23.91% .......................
[25.41 30.54] 16 17.39% .................
[30.71 37.37] 19 20.65% ....................

Responses by status code
[ 200 ] : 10000
```