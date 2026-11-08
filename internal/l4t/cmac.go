package l4t

import (
	"crypto/aes"
	"crypto/subtle"
)

// cmac is AES-CMAC (RFC 4493) over msg with a 128-bit key.
func cmac(key [16]byte, msg []byte) [16]byte {
	block, _ := aes.NewCipher(key[:]) // a 16-byte key never errors

	var zero, k1, k2 [16]byte
	block.Encrypt(k1[:], zero[:])
	k1 = double(k1)
	k2 = double(k1)

	n := (len(msg) + 15) / 16
	complete := n > 0 && len(msg)%16 == 0
	if n == 0 {
		n = 1
	}

	var last [16]byte
	tail := msg[(n-1)*16:]
	copy(last[:], tail)
	if complete {
		xor(&last, k1)
	} else {
		last[len(tail)] = 0x80
		xor(&last, k2)
	}

	var x [16]byte
	for i := 0; i < n-1; i++ {
		var chunk [16]byte
		copy(chunk[:], msg[i*16:])
		xor(&x, chunk)
		block.Encrypt(x[:], x[:])
	}
	xor(&x, last)
	block.Encrypt(x[:], x[:])
	return x
}

func xor(dst *[16]byte, src [16]byte) {
	subtle.XORBytes(dst[:], dst[:], src[:])
}

func double(b [16]byte) [16]byte {
	var out [16]byte
	for i := 0; i < 16; i++ {
		out[i] = b[i] << 1
		if i < 15 {
			out[i] |= b[i+1] >> 7
		}
	}
	if b[0]&0x80 != 0 {
		out[15] ^= 0x87
	}
	return out
}
