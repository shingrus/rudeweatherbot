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

const WEATHER_UPDATE_INTERVAL_SEC = 7200
const apiUrl = "http://dataservice.accuweather.com/forecasts/v1/daily/1day/294021?apikey=%s&metric=true"

type WatherForecast struct {
	mut             sync.Mutex
	RainPrediction  int32 //0 - 3
	CloudPrediction int32 //0 -3 from bad to good
	updateTime      time.Time
}

func (forecast *WatherForecast) getWeatherEveryNsec(N uint64 /*, b *tb.Bot,  chats *Chats*/) {

	for {
		forecast.updateAccuWeather()

		//wake up every N minutes
		time.Sleep(time.Second * time.Duration(N))

	}
}

const TEXT_DEFAULT = "Отъебись, не знаю"

//const TEXT_CLOUD_GOOD = "За окном  заебца, можешь ебануть пивчанского"
var TEXTS_CLOUD_GOOD = []string{"За окном  заебца, можешь ебануть пивчанского", "Там пиздато, лучше только в запое", "Сегодня  будет охуительно"}
var TEXTS_CLOUD_MEH = []string{"На улице хуево, лучше накатить коньячку.", "Какая-то блядская сегодня погода, предлагаю вискаря."}
var TEXTS_CLOUD_BAD = []string{"За окном пизда, займи и выпей водки.", "Оч хуево сегодня, отправь гонца за хмурым."}

const TEXT_RAIN_MEH = " С неба может пиздануть, нехуй там делать"
const TEXT_RAIN_BAD = " Кстати, не проеби зонт."

func (forecast *WatherForecast) isFresh() (fresh bool) {

	log.Printf("Is weather fresh: %t", time.Since(forecast.updateTime).Hours() < 6)
	return time.Since(forecast.updateTime).Hours() < 6

}

func (forecast *WatherForecast) GetRudeForecast() (text string) {
	text = TEXT_DEFAULT

	switch forecast.CloudPrediction {
	case 3:
		text = TEXTS_CLOUD_GOOD[rand.Intn(len(TEXTS_CLOUD_GOOD))]
	case 2:
		text = TEXTS_CLOUD_MEH[rand.Intn(len(TEXT_RAIN_MEH))]
	case 1:
		text = TEXTS_CLOUD_BAD[rand.Intn(len(TEXT_RAIN_BAD))]
	}

	switch forecast.RainPrediction {
	case 1:
		text += TEXT_RAIN_MEH
	case 2:
		text += TEXT_RAIN_BAD
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
		log.Printf("Fetch weather error: %n %s", res.StatusCode, res.Status)
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
