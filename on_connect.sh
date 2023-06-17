#!/bin/bash
ip link set $1 up
ip addr add 192.168.213.5/32 dev $1
