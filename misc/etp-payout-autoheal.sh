#!/usr/bin/env bash

PAYMENTS="etp:payments"
SERVICE="oep-etp-payouts"

systemctl -n 1 status $SERVICE | grep -i "payments suspended"
if [ $? -eq 0 ]; then
    grep -i "(empty list or set)" <<< $(redis-cli --no-raw ZREVRANGE "$PAYMENTS:pending" 0 -1 WITHSCORES)
    if [ $? -eq 0 ]; then
        redis-cli DEL "$PAYMENTS:lock"
        systemctl restart $SERVICE
    fi
fi