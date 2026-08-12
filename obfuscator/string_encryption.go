package obfuscator

import "encoding/base64"

// decryptStrategy 字符串解密策略，每种策略对应生成一个独立的解密函数。
// 多样化的策略使单次识别无法批量解密全部字符串。
type decryptStrategy int

const (
	strategyXOR    decryptStrategy = 0 // 纯 XOR
	strategyXORAdd decryptStrategy = 1 // XOR + 索引加法
	strategyXORRot decryptStrategy = 2 // 字节循环移位 + XOR
)

// numDecryptStrategies 支持的策略数量
const numDecryptStrategies = 3

// rotateLeft8 循环左移 8 位字节
func rotateLeft8(b byte, r uint) byte {
	r %= 8
	return (b << r) | (b >> (8 - r))
}

// rotateRight8 循环右移 8 位字节
func rotateRight8(b byte, r uint) byte {
	r %= 8
	return (b >> r) | (b << (8 - r))
}

// encryptStringWithStrategy 使用指定策略加密字符串，返回 base64 编码的密文。
func (o *Obfuscator) encryptStringWithStrategy(text string, strategy decryptStrategy) string {
	key := o.encryptionKey
	textBytes := []byte(text)
	encryptedBytes := make([]byte, len(textBytes))
	kl := len(key)

	for i, b := range textBytes {
		k := key[i%kl]
		switch strategy {
		case strategyXOR:
			encryptedBytes[i] = b ^ k
		case strategyXORAdd:
			encryptedBytes[i] = (b ^ k) + byte(i&0xff)
		case strategyXORRot:
			encryptedBytes[i] = rotateLeft8(b, uint(k)) ^ k
		default:
			encryptedBytes[i] = b ^ k
		}
	}

	return base64.StdEncoding.EncodeToString(encryptedBytes)
}
