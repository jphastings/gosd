# GoSD

Turn Go applications into SD card images for the Raspberry Pi Zero 2w and the Radxa Zero 3E.

Like GoKrazy, but the result is something _anyone_ can burn and use.

## Features

- Simple CLI tool that can be run locally or in CI
- Extremely fast boot (under 5 seconds, including Wifi)
- Optional USB OTG (run as a USB _device_)
- Connect to the internet via Ethernet (assumes DHCP) or WiFi (credentials added as your SD card is written)
- Run any normal (linux-capable) Go application
