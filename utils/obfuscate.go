/*
 * // Copyright (C) 2017 Yeecall authors
 * //
 * // This file is part of the Yecall library.
 *
 */

package utils

/*
  混淆算法：随机生成一个2字节混淆头，然后用2字节混淆头+data len计算出一个偏移位置，然后利用obfDict中的偏移数据进行一次xor
 */

func ObfuscateData(data []byte) []byte {
	return data
}

func DataFromObfuscated(obf []byte) []byte {
	return obf
}


var obfDict = []byte{
	0x01, 0x02,
}