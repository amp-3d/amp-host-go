#!/bin/bash
TIMESTAMP=`date +"%Y-%m-%d-%H-%MUTC"`
START=$(date +%s)

cd /opt/plan/go-arcgate
service arcgate stop
sleep 2
git reset --hard
sleep 1
git pull
go build -trimpath -o cmd/arcgate
service arcgate start
service arcgate status
END=$(date +%s)
DIFF=$(( $END - $START ))
echo
echo "go-arcgate refresh lasted $DIFF seconds"
echo