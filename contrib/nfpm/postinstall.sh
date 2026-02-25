#!/bin/sh
udevadm control --reload-rules
udevadm trigger
systemctl --global enable z13gui.service || true
