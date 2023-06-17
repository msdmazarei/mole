#!/bin/bash
ip link set $1 down
ip addr del 192.168.213.5/32 dev $1
