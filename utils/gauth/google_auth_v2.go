package gauth

import (
	"bytes"
	"github.com/godaddy-x/freego/utils"
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
	return "data:image/png;base64," + utils.Base64Encode(buf.Bytes()), nil
}

func Create(issuer, accountName string, size int) (string, string, error) {
	secret, url, err := CreateURL(issuer, accountName)
	if err != nil {
		return "", "", err
	}
	image, err := CreateImage(url, size)
	if err != nil {
		return "", "", err
	}
	return secret, image, nil
}

// CreateURL 密钥,链接
func CreateURL(issuer, accountName string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func CreateImage(url string, size int) (string, error) {
	image, err := getQRImage(url, size)
	if err != nil {
		return "", err
	}
	return image, nil
}

func Validate(code, secret string) bool {
	if len(secret) == 0 || len(code) != 6 {
		return false
	}
	return totp.Validate(code, secret)
}
