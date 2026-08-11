---
error_code: GOSD-APP-CRASH
timestamp: 2026-09-11T11:57:03Z
clock: ntp-synced
uptime: 4m12s
boot: 37
device: Raspberry Pi Zero 2 W Rev 1.0 (pi-zero-2w)
image: "myapp 0.1.0 #a1b2c3d4"
---

# myapp crash report

Your myapp device stopped while running.

This file was written by the device itself, onto its own SD card, so you can
read it on any computer. Nothing was sent anywhere.

## The problem

The app stopped unexpectedly.

## The fix

We don't have a specific fix for this one. Visit https://example.com/support and
quote the error code above.

## What to send

If you ask anyone for help, send them **this whole file** rather than a
summary — the section below is the part they need.

## Technical detail

    panic: runtime error: invalid memory address or nil pointer dereference
    [signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x5f2a4]

    goroutine 1 [running]:
    main.(*sampler).read(0x400012c000, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:100 +0x1c
    goroutine 2 [running]:
    main.(*sampler).read(0x400012c001, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:101 +0x1d
    goroutine 3 [running]:
    main.(*sampler).read(0x400012c002, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:102 +0x1e
    goroutine 4 [running]:
    main.(*sampler).read(0x400012c003, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:103 +0x1f
    goroutine 5 [running]:
    main.(*sampler).read(0x400012c004, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:104 +0x20
    goroutine 6 [running]:
    main.(*sampler).read(0x400012c005, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:105 +0x21
    goroutine 7 [running]:
    main.(*sampler).read(0x400012c006, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:106 +0x22
    goroutine 8 [running]:
    main.(*sampler).read(0x400012c007, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:107 +0x23
    goroutine 9 [running]:
    main.(*sampler).read(0x400012c008, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:108 +0x24
    goroutine 10 [running]:
    main.(*sampler).read(0x400012c009, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:109 +0x25
    goroutine 11 [running]:
    main.(*sampler).read(0x400012c00a, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:110 +0x26
    goroutine 12 [running]:
    main.(*sampler).read(0x400012c00b, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:111 +0x27
    goroutine 13 [running]:
    main.(*sampler).read(0x400012c00c, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:112 +0x28
    goroutine 14 [running]:
    main.(*sampler).read(0x400012c00d, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:113 +0x29
    goroutine 15 [running]:
    main.(*sampler).read(0x400012c00e, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:114 +0x2a
    goroutine 16 [running]:
    main.(*sampler).read(0x400012c00f, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:115 +0x2b
    goroutine 17 [running]:
    main.(*sampler).read(0x400012c010, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:116 +0x2c
    goroutine 18 [running]:
    main.(*sampler).read(0x400012c011, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:117 +0x2d
    goroutine 19 [running]:
    main.(*sampler).read(0x400012c012, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:118 +0x2e
    goroutine 20 [running]:
    main.(*sampler).read(0x400012c013, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:119 +0x2f
    goroutine 21 [running]:
    main.(*sampler).read(0x400012c014, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:120 +0x30
    goroutine 22 [running]:
    main.(*sampler).read(0x400012c015, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:121 +0x31
    goroutine 23 [running]:
    main.(*sampler).read(0x400012c016, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:122 +0x32
    goroutine 24 [running]:
    main.(*sampler).read(0x400012c017, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:123 +0x33
    goroutine 25 [running]:
    main.(*sampler).read(0x400012c018, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:124 +0x34
    goroutine 26 [running]:
    main.(*sampler).read(0x400012c019, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:125 +0x35
    goroutine 27 [running]:
    main.(*sampler).read(0x400012c01a, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:126 +0x36
    goroutine 28 [running]:
    main.(*sampler).read(0x400012c01b, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:127 +0x37
    goroutine 29 [running]:
    main.(*sampler).read(0x400012c01c, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:128 +0x38
    goroutine 30 [running]:
    main.(*sampler).read(0x400012c01d, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:129 +0x39
    goroutine 31 [running]:
    main.(*sampler).read(0x400012c01e, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:130 +0x3a
    goroutine 32 [running]:
    main.(*sampler).read(0x400012c01f, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:131 +0x3b
    goroutine 33 [running]:
    main.(*sampler).read(0x400012c020, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:132 +0x3c
    goroutine 34 [running]:
    main.(*sampler).read(0x400012c021, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:133 +0x3d
    goroutine 35 [running]:
    main.(*sampler).read(0x400012c022, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:134 +0x3e
    goroutine 36 [running]:
    main.(*sampler).read(0x400012c023, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:135 +0x3f
    goroutine 37 [running]:
    main.(*sampler).read(0x400012c024, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:136 +0x40
    goroutine 38 [running]:
    main.(*sampler).read(0x400012c025, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:137 +0x41
    goroutine 39 [running]:
    main.(*sampler).read(0x400012c026, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:138 +0x42
    goroutine 40 [running]:
    main.(*sampler).read(0x400012c027, {0x4000180000, 0x40, 0x40})
    	/home/dev/myapp/sampler.go:139 +0x43
