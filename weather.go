package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"
)


const llmPrompt = 'imagine you are one of bad russian school student 
answer on user request uses offensive langueage, profanity. 
answer only in russian.
give good advices how to choose clothes or get umbrella 
today is january 14 2024
tomorrow forecast is day temperature -1 celsius, mainly clioudy light wet snow
Use russian mat
Be kind with asking person'

const WEATHER_UPDATE_INTERVAL_SEC = 7200
const apiUrl = "http://dataservice.accuweather.com/forecasts/v1/daily/1day/294021?apikey=%s&metric=true"

type WatherForecast struct {
	mut             sync.Mutex
	RainPrediction  int32 //0 - no rain, 1 - light possible rain, 2 - rain
	CloudPrediction int32 //0 -3 from bad to good
	updateTime      time.Time
}

type WeatherText struct {
	counter int
	texts   []string
}

//
func (wtext *WeatherText) getNextText() string {

	if wtext.counter == -1 {
		wtext.counter = rand.Intn(len(wtext.texts))
	}
	wtext.counter++
	wtext.counter = wtext.counter % len(wtext.texts)
	return wtext.texts[wtext.counter]
}

func (forecast *WatherForecast) getWeatherEveryNsec(N uint64 /*, b *tb.Bot,  chats *Chats*/) {

	for {
		//forecast.updateAccuWeather()
		forecast.updateOpenWeather()

		//wake up every N minutes
		time.Sleep(time.Second * time.Duration(N))

	}
}

const TEXT_DEFAULT = "Отъебись, не знаю."

var TEXTS_CLOUD_GOOD = WeatherText{-1, []string{"За окном  заебца, можешь ебануть пивчанского.", "Там пиздато, лучше только в запое.", "Сегодня будет охуительно.",
	"Сегодня ты - директор пляжа, выдави крема на ебло.", "Жмурься на солнышко с удовольствием, скоро это закончится."}}

var TEXTS_CLOUD_MEH = WeatherText{-1, []string{"На улице хуево, лучше накатить коньячку.", "Какая-то блядская сегодня погода, предлагаю вискаря.",
	"Там все пусто и бессмысленно, мы же в России.", "На улице, как в постели с бывшей, - никак."}}

var TEXTS_CLOUD_BAD = WeatherText{-1, []string{"За окном пизда, займи и выпей водки.", "Оч хуево сегодня, отправь гонца за хмурым.", "За окном Челябинск и Череповец, \"ничего личного, просто пиздец\"."}}

var TEXTS_RAIN_MEH = WeatherText{-1, []string{"С неба может пиздануть, нехуй там делать.", "Возможно ливанет."}}
var TEXTS_RAIN_BAD = WeatherText{-1, []string{"На улицу соберешься - зонт не забудь, блять.", "Будешь выходить - дождевичок накинь, епта."}}

func (forecast *WatherForecast) isFresh() (fresh bool) {

	log.Printf("Is weather fresh: %t", time.Since(forecast.updateTime).Hours() < 6)
	return time.Since(forecast.updateTime).Hours() < 6

}

func (forecast *WatherForecast) GetRudeForecast() (text string) {
	text = TEXT_DEFAULT

	switch forecast.CloudPrediction {
	case 3:
		text = TEXTS_CLOUD_GOOD.getNextText()
	case 2:
		text = TEXTS_CLOUD_MEH.getNextText()
	case 1:
		text = TEXTS_CLOUD_BAD.getNextText()
	}

	switch forecast.RainPrediction {
	case 1:
		text += " " + TEXTS_RAIN_MEH.getNextText()
	case 2:
		text += " " + TEXTS_RAIN_BAD.getNextText()
	}
	return
}

type JSForecast struct {
	DailyForecasts []struct {
		Date        time.Time `json:"Date"`
		EpochDate   int       `json:"EpochDate"`
		Temperature struct {
			Minimum struct {
				Value    float64 `json:"Value"`
				Unit     string  `json:"Unit"`
				UnitType int     `json:"UnitType"`
			} `json:"Minimum"`
			Maximum struct {
				Value    float64 `json:"Value"`
				Unit     string  `json:"Unit"`
				UnitType int     `json:"UnitType"`
			} `json:"Maximum"`
		} `json:"Temperature"`
		Day struct {
			Icon       int    `json:"Icon"`
			IconPhrase string `json:"IconPhrase"`
		} `json:"Day"`
		Night struct {
			Icon       int    `json:"Icon"`
			IconPhrase string `json:"IconPhrase"`
		} `json:"Night"`
		Sources    []string `json:"Sources"`
		MobileLink string   `json:"MobileLink"`
		Link       string   `json:"Link"`
	} `json:"DailyForecasts"`
}

func (forecast *WatherForecast) updateAccuWeather() {
	var myClient = &http.Client{Timeout: 30 * time.Second}

	uri := fmt.Sprintf(apiUrl, os.Getenv(accuWeatherEnvVar))
	res, err := myClient.Get(uri)
	if err == nil && res.StatusCode == 200 {
		dec := json.NewDecoder(res.Body)

		for dec.More() {
			var jval JSForecast
			err := dec.Decode(&jval)
			if err != nil {
				fmt.Println(err)
				break
			}
			if len(jval.DailyForecasts) > 0 {

				forecast.mut.Lock()

				fmt.Printf("Forecast: Day:%s  Night: %s\n", jval.DailyForecasts[0].Day.IconPhrase, jval.DailyForecasts[0].Night.IconPhrase)

				//cloud status
				switch jval.DailyForecasts[0].Day.Icon {
				case 0, 1, 2, 3, 4, 5:
					forecast.CloudPrediction = 3
					forecast.RainPrediction = 0
				case 6, 7, 8, 11, 20, 21, 23:
					forecast.CloudPrediction = 2
					forecast.RainPrediction = 1
				case 12, 13, 14, 15, 16, 17, 18, 29, 22, 24, 25, 26:
					forecast.CloudPrediction = 1
					forecast.RainPrediction = 2
				}

				forecast.updateTime = time.Now()
				forecast.mut.Unlock()

			}

			break
		}
		err = res.Body.Close()
	} else if res != nil && res.StatusCode != 200 {
		log.Printf("Fetch weather error: %d %s", res.StatusCode, res.Status)
	} else {
		log.Println(err)
	}

	return
}

func InintWeather() (forecast *WatherForecast) {
	forecast = &WatherForecast{CloudPrediction: 0, RainPrediction: 0}
	go forecast.getWeatherEveryNsec(WEATHER_UPDATE_INTERVAL_SEC)
	return
}
