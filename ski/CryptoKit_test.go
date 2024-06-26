package ski_test

import (
	"bytes"
	crypto_rand "crypto/rand"
	math_rand "math/rand"

	"testing"

	"github.com/amp-3d/amp-host-go/ski"

	_ "github.com/amp-3d/amp-host-go/ski/ed25519"
	_ "github.com/amp-3d/amp-host-go/ski/nacl"
)

var gTesting *testing.T

func TestCryptoKits(t *testing.T) {

	gTesting = t

	// Register providers to test
	cryptoKitsToTest := []ski.CryptoKitID{
		ski.CryptoKitID_ED25519,
		ski.CryptoKitID_NaCl,
	}

	for _, kitID := range cryptoKitsToTest {
		for i := 0; i < 200; i++ {
			kit, err := ski.GetCryptoKit(kitID)
			if err != nil {
				gTesting.Fatal(err)
			}
			testKit(kit, 32)
		}
	}
}

func testKit(kit ski.CryptoKit, inKeyLen int) {
	msgSz := int(1 + math_rand.Int31n(5) + 7*math_rand.Int31n(10))

	msg := make([]byte, msgSz)
	msgOrig := make([]byte, msgSz)
	badMsg := make([]byte, msgSz)

	crypto_rand.Read(msg)
	copy(msgOrig, msg)

	reader := crypto_rand.Reader

	if bytes.Compare(msgOrig, msg) != 0 {
		gTesting.Fatal("initial msg check failed!?")
	}

	var crypt []byte
	var passBuf [200]byte

	/*****************************************************
	** Symmetric password test
	**/
	{
		passLen := 1 + math_rand.Int31n(30)
		pass := passBuf[:passLen]
		math_rand.Read(pass)

		crypt, err := kit.EncryptUsingPassword(reader, msgOrig, pass)
		if ski.IsError(err, ski.ErrCode_Unimplemented) == false {
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			msg, err = kit.DecryptUsingPassword(crypt, pass)
			if bytes.Compare(msg, msgOrig) != 0 {
				gTesting.Fatal("symmetric decrypt failed check")
			}
		}
	}

	entry := ski.KeyEntry{
		KeyInfo: &ski.KeyInfo{
			CryptoKitID: kit.CryptoKitID(),
		},
	}

	/*****************************************************
	** Symmetric test
	**/
	{
		entry.KeyInfo.KeyType = ski.KeyType_SymmetricKey
		err := kit.GenerateNewKey(inKeyLen, reader, &entry)
		if ski.IsError(err, ski.ErrCode_Unimplemented) == false {
			if err != nil {
				gTesting.Fatal(err)
			}

			crypt, err = kit.Encrypt(reader, msgOrig, entry.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			msg, err = kit.Decrypt(crypt, entry.PrivKey)
			if bytes.Compare(msg, msgOrig) != 0 {
				gTesting.Fatal("symmetric decrypt failed check")
			}

			// Vary the data slightly to test
			for k := 0; k < 100; k++ {

				rndPos := math_rand.Int31n(int32(len(crypt)))
				rndAdj := 1 + byte(math_rand.Int31n(254))
				copy(badMsg, crypt)
				badMsg[rndPos] += rndAdj

				msg, err = kit.Decrypt(badMsg, entry.PrivKey)
				if err == nil {
					gTesting.Fatal("there should have been a decryption error!")
				}
			}
		}
	}

	/*****************************************************
	** Asymmetric test
	**/
	{
		entry.KeyInfo.KeyType = ski.KeyType_AsymmetricKey
		err := kit.GenerateNewKey(inKeyLen, reader, &entry)
		if ski.IsError(err, ski.ErrCode_Unimplemented) == false {
			if err != nil {
				gTesting.Fatal(err)
			}

			recipient := ski.KeyEntry{
				KeyInfo: &ski.KeyInfo{},
			}
			*recipient.KeyInfo = *entry.KeyInfo

			err = kit.GenerateNewKey(inKeyLen, reader, &recipient)
			if err != nil {
				gTesting.Fatal(err)
			}

			crypt, err = kit.EncryptFor(reader, msgOrig, recipient.KeyInfo.PubKey, entry.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			msg, err = kit.DecryptFrom(crypt, entry.KeyInfo.PubKey, recipient.PrivKey)
			if bytes.Compare(msg, msgOrig) != 0 {
				gTesting.Fatal("asymmetric decrypt failed check")
			}

			// Vary the data slightly to test
			for k := 0; k < 100; k++ {

				rndPos := math_rand.Int31n(int32(len(crypt)))
				rndAdj := 1 + byte(math_rand.Int31n(254))
				copy(badMsg, crypt)
				badMsg[rndPos] += rndAdj

				msg, err = kit.DecryptFrom(badMsg, entry.KeyInfo.PubKey, recipient.PrivKey)
				if err == nil {
					gTesting.Fatal("there should have been a decryption error!")
				}
			}
		}
	}

	/*****************************************************
	** Signing test
	**/
	{
		entry.KeyInfo.KeyType = ski.KeyType_SigningKey
		err := kit.GenerateNewKey(inKeyLen, reader, &entry)
		if ski.IsError(err, ski.ErrCode_Unimplemented) == false {
			if err != nil {
				gTesting.Fatal(err)
			}

			crypt, err = kit.Sign(msgOrig, entry.PrivKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			err = kit.VerifySignature(crypt, msgOrig, entry.KeyInfo.PubKey)
			if err != nil {
				gTesting.Fatal(err)
			}

			if len(badMsg) != len(crypt) {
				badMsg = make([]byte, len(crypt))
			}

			// Vary the data slightly to test
			for k := 0; k < 100; k++ {

				rndPos := math_rand.Int31n(int32(len(crypt)))
				rndAdj := 1 + byte(math_rand.Int31n(254))
				copy(badMsg, crypt)
				badMsg[rndPos] += rndAdj

				err = kit.VerifySignature(badMsg, msgOrig, entry.KeyInfo.PubKey)
				if ski.IsError(err, ski.ErrCode_VerifySignatureFailed) == false {
					gTesting.Fatal("there should have been a sig failed error!")
				}
			}
		}
	}
}
