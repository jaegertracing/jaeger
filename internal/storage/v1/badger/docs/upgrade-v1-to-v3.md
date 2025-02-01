# Upgrade Badger v1 to v3

In Jaeger 1.24.0, Badger is upgraded from v1.6.2 to v3.2103.0 which changes the underlying data format. Following steps will help in migrating your data:

1. In Badger v1, the data looks like:

```sh
❯ ls /tmp/badger/
data  key
❯ ls /tmp/badger/data/
000001.vlog  000004.vlog  000005.vlog  000008.vlog  000011.vlog  000012.vlog  000013.vlog  000014.vlog  000015.vlog  000016.vlog  000017.vlog
❯ ls /tmp/badger/key/
000038.sst  000048.sst  000049.sst  000050.sst  000051.sst  000059.sst  000060.sst  000061.sst  000063.sst  000064.sst  000065.sst  000066.sst  MANIFEST
```

2. Make a backup of your data directory to have a copy incase migration didn't work successfully.

```sh
❯ cp -r /tmp/badger /tmp/badger.bk
```

3. Download, extract and compile the source code of badger v1: https://github.com/dgraph-io/badger/archive/refs/tags/v1.6.2.tar.gz

```sh
❯ tar xvzf badger-1.6.2.tar
❯ cd badger-1.6.2/badger/
❯ go install
```

This will install the badger command line utility into your $GOBIN path eg ~/go/bin/badger.

4. Use badger utility to take backup of data.

```sh
❯ ~/go/bin/badger backup --dir /tmp/badger/key --vlog-dir /tmp/badger/data/
Listening for /debug HTTP requests at port: 8080
badger 2021/06/24 22:04:30 INFO: All 12 tables opened in 907ms
badger 2021/06/24 22:04:30 INFO: Replaying file id: 17 at offset: 64584535
badger 2021/06/24 22:04:30 INFO: Replay took: 12.303µs
badger 2021/06/24 22:04:30 DEBUG: Value log discard stats empty
badger 2021/06/24 22:04:30 INFO: DB.Backup Created batch of size: 9.7 kB in 75.907µs.
badger 2021/06/24 22:04:31 INFO: DB.Backup Created batch of size: 4.3 MB in 8.003592ms.
....
....
badger 2021/06/24 22:04:31 INFO: DB.Backup Created batch of size: 30 MB in 74.808075ms.
badger 2021/06/24 22:04:36 INFO: DB.Backup Sent 15495232 keys
badger 2021/06/24 22:04:36 INFO: Got compaction priority: {level:0 score:1.73 dropPrefixes:[]}
```

This will create a badger.bak file in the current directory.

5. Download, extract and compile the source code of badger v3: https://github.com/dgraph-io/badger/archive/refs/tags/v3.2103.0.tar.gz

```sh
❯ tar xvzf badger-3.2103.0.tar
❯ cd badger-3.2103.0/badger/
❯ go install
```

This will install the badger command line utility into your $GOBIN path eg ~/go/bin/badger.

6. Restore the data from backup.

```sh
❯ ~/go/bin/badger restore --dir jaeger-v3
Listening for /debug HTTP requests at port: 8080
jemalloc enabled: false
Using Go memory
badger 2021/06/24 22:08:29 INFO: All 0 tables opened in 0s
badger 2021/06/24 22:08:29 INFO: Discard stats nextEmptySlot: 0
badger 2021/06/24 22:08:29 INFO: Set nextTxnTs to 0
badger 2021/06/24 22:08:37 INFO: [0] [E] LOG Compact 0->6 (5, 0 -> 50 tables with 1 splits). [00001 00002 00003 00004 00005 . .] -> [00006 00007 00008 00009 00010 00011 00012 00013 00014 00015 00016 00017 00018 00019 00020 00021 00022 00023 00024 00025 00026 00028 00029 00030 00031 00032 00033 00034 00035 00036 00037 00038 00039 00040 00041 00043 00044 00045 00046 00047 00048 00049 00050 00051 00052 00053 00054 00055 00056 00057 .], took 2.597s
badger 2021/06/24 22:08:53 INFO: Lifetime L0 stalled for: 0s
badger 2021/06/24 22:08:55 INFO:
Level 0 [ ]: NumTables: 00. Size: 0 B of 0 B. Score: 0.00->0.00 StaleData: 0 B Target FileSize: 64 MiB
Level 1 [ ]: NumTables: 00. Size: 0 B of 10 MiB. Score: 0.00->0.00 StaleData: 0 B Target FileSize: 2.0 MiB
Level 2 [ ]: NumTables: 00. Size: 0 B of 10 MiB. Score: 0.00->0.00 StaleData: 0 B Target FileSize: 2.0 MiB
Level 3 [ ]: NumTables: 00. Size: 0 B of 10 MiB. Score: 0.00->0.00 StaleData: 0 B Target FileSize: 2.0 MiB
Level 4 [B]: NumTables: 45. Size: 86 MiB of 10 MiB. Score: 8.64->10.21 StaleData: 0 B Target FileSize: 2.0 MiB
Level 5 [ ]: NumTables: 08. Size: 29 MiB of 34 MiB. Score: 0.00->0.00 StaleData: 0 B Target FileSize: 4.0 MiB
Level 6 [ ]: NumTables: 63. Size: 340 MiB of 340 MiB. Score: 0.00->0.00 StaleData: 0 B Target FileSize: 8.0 MiB
Level Done
Num Allocated Bytes at program end: 0 B
```

This will restore the data in jaeger-v3 directory. It will look like this

```sh
❯ ls ./jaeger-v3
000001.vlog  000180.sst  000257.sst  000276.sst  000294.sst  000327.sst  000336.sst  000349.sst  000356.sst  000364.sst  000371.sst  000378.sst  000385.sst  000392.sst  000399.sst  000406.sst  000413.sst   MANIFEST
000006.sst   000181.sst  000259.sst  000277.sst  000302.sst  000328.sst  000339.sst  000350.sst  000357.sst  000365.sst  000372.sst  000379.sst  000386.sst  000393.sst  000400.sst  000407.sst  000414.sst
000007.sst   000195.sst  000261.sst  000278.sst  000305.sst  000330.sst  000340.sst  000351.sst  000359.sst  000366.sst  000373.sst  000380.sst  000387.sst  000394.sst  000401.sst  000408.sst  000415.sst
000008.sst   000218.sst  000265.sst  000279.sst  000315.sst  000331.sst  000341.sst  000352.sst  000360.sst  000367.sst  000374.sst  000381.sst  000388.sst  000395.sst  000402.sst  000409.sst  000416.sst
000061.sst   000227.sst  000267.sst  000282.sst  000324.sst  000332.sst  000343.sst  000353.sst  000361.sst  000368.sst  000375.sst  000382.sst  000389.sst  000396.sst  000403.sst  000410.sst  000417.sst
000134.sst   000249.sst  000272.sst  000285.sst  000325.sst  000333.sst  000344.sst  000354.sst  000362.sst  000369.sst  000376.sst  000383.sst  000390.sst  000397.sst  000404.sst  000411.sst  DISCARD
000154.sst   000255.sst  000275.sst  000289.sst  000326.sst  000334.sst  000348.sst  000355.sst  000363.sst  000370.sst  000377.sst  000384.sst  000391.sst  000398.sst  000405.sst  000412.sst  KEYREGISTRY
```

7. Separate out the key and data directories.

```sh
❯ rm -rf /tmp/badger
❯ mv ./jaeger-v3 /tmp/badger
❯ mkdir /tmp/badger/data /tmp/badger/key
❯ mv /tmp/badger/*.vlog /tmp/badger/data/
❯ mv /tmp/badger/*.sst /tmp/badger/key/
❯ mv /tmp/badger/MANIFEST /tmp/badger/DISCARD /tmp/badger/KEYREGISTRY /tmp/badger/key/
```

8. Start Jaeger v1.24.0. It should start well.
