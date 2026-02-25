#!/bin/sh
udevadm control --reload-rules
udevadm trigger
systemctl --global enable z13ctl.socket z13ctl.service || true
systemctl --global enable z13gui.service || true
