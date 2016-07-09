#! /bin/bash

cp init /etc/init.d/go-monitor
chmod +x /etc/init.d/go-monitor
go build
cp go-monitor /usr/local/bin/
cp go-monitor.yml /usr/local/etc/

update-rc.d go-monitor defaults
