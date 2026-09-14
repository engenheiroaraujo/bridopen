package captcha

import "github.com/mojocn/base64Captcha"

func GenerateCaptcha() (id string, value string, err error) {

	/*
		NewDriverDigit(
			100,  altura da imagem em pixels
			200,  largura da imagem em pixels
			4,    quantidade de números no captcha
			0,    inclinação/distorção dos números
			100,  quantidade de pontos de ruído na imagem
		)
	*/
	driver := base64Captcha.NewDriverDigit(90, 220, 6, 0.75, 140)
	captcha := base64Captcha.NewCaptcha(driver, base64Captcha.DefaultMemStore)
	id, value, _, err = captcha.Generate()
	return
}

func ValidateCaptcha(id string, answer string) bool {
	return base64Captcha.DefaultMemStore.Verify(id, answer, true)
}
