#!/bin/bash
ip link set $1 down
ip addr del 192.168.213.10/24 dev $1
