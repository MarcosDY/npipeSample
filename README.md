# npipeSample

## Tests I ran

### Local: 
```
go run server\main.go

2022/04/04 17:17:56 Listening on \\.\pipe\wservice
2022/04/04 17:18:07 ProcessID: 22300
```

```
go run .\client\main.go
2022/04/04 17:18:07 PID: 22300
2022/04/04 17:18:07 SPIFFEID: "someID"
```

### Docker:

Start container:
```
docker run  -v \\.\pipe\wservice:\\.\pipe\wservice --network=externalSwitch  --hostname=tete -it mcr.microsoft.com/windows/servercore:ltsc2022  powershell
```

```
PS C:\npipe> .\server.exe
2022/04/04 12:55:03 Listening on \\.\pipe\wservice
2022/04/04 12:59:22 ProcessID: 1244
```

```
PS C:\> .\client.exe
2022/04/04 12:59:22 PID: 1244
2022/04/04 12:59:22 SPIFFEID: "someID"
```

### K8s

Create cluster:
```
eksctl create cluster -f cluster/cluster.yaml
```


