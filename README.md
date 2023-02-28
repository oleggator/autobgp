## Mikrotik config example
```
/routing bgp connection
add as=64512 connect=yes input.ignore-as-path-len=yes name=bgp1 remote.address=192.168.88.247 remote.as=65432 router-id=192.168.88.1
```
