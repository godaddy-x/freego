package gauth

import (
	"bytes"
	"encoding/base64"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"image/png"
)

func getQRImage(url string, size int) (string, error) {
	qrCode, err := qrcode.New(url, qrcode.Highest)
	if err != nil {
		return "", err
	}
	return getBase64Image(qrCode, size)
}

func getBase64Image(image *qrcode.QRCode, size int) (string, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, image.Image(size))
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func Create(issuer, accountName string, size int) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", err
	}
	url := key.URL()
	image, err := getQRImage(url, size)
	if err != nil {
		return "", "", err
	}
	return key.Secret(), image, nil
}

func Validate(code, secret string) bool {
	if len(secret) == 0 || len(code) != 6 {
		return false
	}
	return totp.Validate(code, secret)
}
