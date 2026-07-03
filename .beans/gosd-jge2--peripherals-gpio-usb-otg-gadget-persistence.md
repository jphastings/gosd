---
# gosd-jge2
title: 'Peripherals: GPIO, USB OTG gadget, persistence'
status: todo
type: epic
created_at: 2026-07-02T20:50:27Z
updated_at: 2026-07-02T20:50:27Z
parent: gosd-p3zw
---

The 'Go hardware application' capabilities: GPIO/I2C/SPI access from the user app, USB OTG gadget modes (device presents as USB serial / USB Ethernet), and a writable data partition.

Locked decisions: GPIO via the character device (/dev/gpiochipN) using github.com/warthog618/go-gpiocdev — never sysfs. I2C/SPI via periph.io or direct /dev nodes; document, don't wrap. USB gadget via configfs + libcomposite (kernel configs already enabled by the v0.1 kernel tasks: dwc2 on Pi, dwc3 on RK3566); we ship a small pure-Go configfs gadget library as part of the gosd runtime package — this is a headline feature, design its API carefully.
