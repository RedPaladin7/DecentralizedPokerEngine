package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

const p2048Hex = "ffffffffffffffffc90fdaa22168c234c4c6628b80dc1cd1" +
	"29024e088a67cc74020bbea63b139b22514a08798e3404dd" +
	"ef9519b3cd3a431b302b0a6df25f14374fe1356d6d51c245" +
	"e485b576625e7ec6f44c42e9a637ed6b0bff5cb6f406b7ed" +
	"ee386bfb5a899fa5ae9f24117c4b1fe649286651ece45b3d" +
	"c2007cb8a163bf0598da48361c55d39a69163fa8fd24cf5f" +
	"83655d23dca3ad961c62f356208552bb9ed529077096966d" +
	"670c354e4abc9804f1746c08ca18217c32905e462e36ce3b" +
	"e39e772c180e86039b2783a2ec07a28fb5c55df06f4c52c9" +
	"de2bcbf6955817183995497cea956ae515d2261898fa0510" +
	"15728e5a8aacaa68ffffffffffffffff"

func SharedPrime() *big.Int {
	p := new(big.Int)
	p.SetString(p2048Hex, 16)
	return p 
}

func CardToField(cardID int, p *big.Int) *big.Int {
	if cardID < 0 || cardID > 51 {
		panic(fmt.Sprintf(""))
	}
	g := big.NewInt(2)
	exp := big.NewInt(int64(cardID+1))
	return new(big.Int).Exp(g, exp, p)
}

func FieldToCard(val *big.Int, p *big.Int) int {
	g := big.NewInt(2)
	for id := 0; id <= 51; id++ {
		exp := big.NewInt(int64(id+1))
		candidate := new(big.Int).Exp(g, exp, p)
		if candidate.Cmp(val) == 0 {
			return id 
		}
	}
	return -1
}

func BuildPlaintextDeck(p *big.Int) []*big.Int {
	deck := make([]*big.Int, 52)
	for i := range deck {
		deck[i] = CardToField(i, p)
	}
	return deck 
}

func SessionID(playerIDs []string, nonce []byte) []byte {
	h := sha256.New()
	for _, id := range playerIDs {
		h.Write([]byte(id))
		h.Write([]byte{0x00})
	}
	h.Write(nonce)
	return h.Sum(nil)
}

func SessionIDHex(playerIDs []string, nonce []byte) string {
	return hex.EncodeToString(SessionID(playerIDs, nonce))
}