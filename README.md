# Boottime

**Boottime** is a Linux tool that collects and compares system boot times from
multiple sources.

## Requirements

- Go
- systemd
- systemd-analyze

## Usage

### Collect boot time records

Use the `-R` flag to collect boot time data from the available sources. The
results are appended to a `.jsonl` file (one record per boot). If the file does
not exist, it is created automatically.

```console
$ go run ./cmd/boottime -R results.jsonl
$ cat baseline.jsonl 
{"firmware":{"efi_var":1702811000,"systemd_analyze":1708000000,"systemd_dbus":1708265000},"initrd":{"systemd_analyze":200000000,"systemd_dbus":200300000},"kernel":{"systemd_analyze":641000000,"systemd_dbus":641348000},"loader":{"efi_var":151520000,"systemd_analyze":267000000,"systemd_dbus":267711000},"total":{"systemd_analyze":4605000000,"systemd_dbus":4605013000},"userspace":{"systemd_analyze":1787000000,"systemd_dbus":1787389000}}
{"firmware":{"efi_var":1705254000,"systemd_analyze":1710000000,"systemd_dbus":1710756000},"initrd":{"systemd_analyze":210000000,"systemd_dbus":210447000},"kernel":{"systemd_analyze":641000000,"systemd_dbus":641943000},"loader":{"efi_var":149803000,"systemd_analyze":265000000,"systemd_dbus":265373000},"total":{"systemd_analyze":4661000000,"systemd_dbus":4661871000},"userspace":{"systemd_analyze":1833000000,"systemd_dbus":1833352000}}
{"firmware":{"efi_var":1746628000,"systemd_analyze":1752000000,"systemd_dbus":1752035000},"initrd":{"systemd_analyze":181000000,"systemd_dbus":181816000},"kernel":{"systemd_analyze":641000000,"systemd_dbus":641537000},"loader":{"efi_var":146862000,"systemd_analyze":262000000,"systemd_dbus":262381000},"total":{"systemd_analyze":4565000000,"systemd_dbus":4565063000},"userspace":{"systemd_analyze":1727000000,"systemd_dbus":1727294000}}
```

### Average boot time records

Use the `-A` flag to compute the average boot times from an existing `.jsonl`
file. By default, the aggregate result is printed in JSON to stdout.

```console
$ go run ./cmd/boottime -A results.jsonl
{"Values":{"firmware":{"efi_var":1718231000,"systemd_analyze":1723333333,"systemd_dbus":1723685333},"initrd":{"systemd_analyze":197000000,"systemd_dbus":197521000},"kernel":{"systemd_analyze":641000000,"systemd_dbus":641609333},"loader":{"efi_var":149395000,"systemd_analyze":264666666,"systemd_dbus":265155000},"total":{"systemd_analyze":4610333333,"systemd_dbus":4610649000},"userspace":{"systemd_analyze":1782333333,"systemd_dbus":1782678333}}}
```

For a more readable, tabular output, combine `-A` with the `-p` flag:

```console
$ go run ./cmd/boottime/main.go -A -p baseline.jsonl 
Stage      efi_var    systemd_dbus  systemd_analyze  
firmware   1.718231s  1.723685333s  1.723333333s     
loader     149.395ms  265.155ms     264.666666ms     
kernel                641.609333ms  641ms            
initrd                197.521ms     197ms            
userspace             1.782678333s  1.782333333s     
total                 4.610649s     4.610333333s  
```
