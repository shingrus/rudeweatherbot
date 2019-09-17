package main

import (
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	tb "gopkg.in/tucnak/telebot.v2"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
	//"github.com/boltdb/bolt"
)

const tokenEnvVar = "TELETOKEN"

const accuWeatherEnvVar = "ACCUWEATHERAPI"
const openWeatherEnvVar = "OPENWEATHERAPI"

const hourToSendEnvVar = "HOURTOSEND"

const databaseName = "users.db"
const chatsBucket = "chats"
const sendDateBucket = "sendDateBucket"
const sendDateKey = "sendDateKey"

const TEXT_START = "Здарова, @%s, теперь я буду хуярить в тебя погодой каждое утро"
const TEXT_STOP = "@%s, заебало тебе писать, все равно не читаешь"
const TEXT_INFO = "Это погодный бот. Он отправляет прогноз погоды в Москве каждое утро в 9:00. Источник погоды: https://openweathermap.org/find?q=%D0%BC%D0%BE%D1%81%D0%BA%D0%B2%D0%B0"

const DEFAULT_HOUR_TOSEND = 23

type Chats struct {
	chatsMap map[int64]tb.Chat
	mut      sync.Mutex
}

func (chats *Chats) AddChat(newchat tb.Chat) {
	chats.mut.Lock()
	defer chats.mut.Unlock()
	fmt.Printf("Add chat: %s\n", newchat.Username)
	if _, ok := chats.chatsMap[newchat.ID]; !ok {
		chats.chatsMap[newchat.ID] = newchat
		db, err := bolt.Open(databaseName, 0600, nil)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		err = db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(chatsBucket))
			val, _ := json.Marshal(newchat)
			err := b.Put([]byte(strconv.FormatInt(newchat.ID, 10)), val)
			return err
		})

	}
}

func (chats *Chats) RemoveChat(id int64) {
	chats.mut.Lock()
	defer chats.mut.Unlock()
	log.Printf("Removed chat %d", id)
	delete(chats.chatsMap, id)
	db, err := bolt.Open(databaseName, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(chatsBucket))
		err := b.Delete([]byte(strconv.FormatInt(id, 10)))
		return err
	})

}

var _lastSendDate time.Time

func getLastSendDate() time.Time {
	if _lastSendDate.IsZero() {
		db, err := bolt.Open(databaseName, 0600, nil)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		err = db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(sendDateBucket))
			val := b.Get([]byte(sendDateKey))
			if val != nil {
				log.Printf("saved time: %s\n", string(val))
				_lastSendDate, err = time.Parse(time.UnixDate, string(val))
				if err != nil {
					log.Println(err)
					_lastSendDate = time.Now()
					return err
				}
			}
			return nil
		})
	}
	return _lastSendDate
}
func updateLastSendDate(sendDate time.Time) {
	db, err := bolt.Open(databaseName, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(sendDateBucket))
		err := b.Put([]byte(sendDateKey), []byte(sendDate.Format(time.UnixDate)))
		log.Printf("Saved: %s", sendDate.Format(time.UnixDate))
		return err
	})
	_lastSendDate = sendDate
}

func (chats *Chats) getChats() (ret []tb.Chat) {
	chats.mut.Lock()
	defer chats.mut.Unlock()
	for _, v := range chats.chatsMap {
		ret = append(ret, v)
	}
	return
}

func (chats *Chats) SendToAllChatsDaily(b *tb.Bot, forecast *WatherForecast, force bool) {

	hourToSend, err := strconv.Atoi(os.Getenv(hourToSendEnvVar))
	if err != nil || hourToSend == 0 {
		hourToSend = DEFAULT_HOUR_TOSEND
	}

	for {
		now := time.Now()
		hours, _, _ := now.Clock()

		if force || hours == hourToSend {
			//check if we already sent today
			lastSendDate := getLastSendDate()
			fmt.Printf("Time diff in hours: %f and force is %t\n", time.Since(lastSendDate).Hours(), force)
			if force || (forecast.isFresh() && time.Since(lastSendDate).Hours() > 23) {
				text := forecast.GetRudeForecast()
				for _, chat := range chats.getChats() {
					_, err := b.Send(&chat, text)
					if err != nil {
						switch err.Error() {
						case "api error: Bad Request: chat not found":
							chats.RemoveChat(chat.ID)
						default:
							fmt.Println(err)
						}
					}
				}
				if !force {
					updateLastSendDate(now)
				}
			}
		} else {
			log.Printf("Time hours(%d), not the time to send(%d)", hours, hourToSend)
		}
		time.Sleep(time.Second * 62)
	}
}

func InitChats() (chats *Chats) {
	chats = &Chats{chatsMap: make(map[int64]tb.Chat)}
	db, err := bolt.Open(databaseName, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(chatsBucket))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		_, err = tx.CreateBucketIfNotExists([]byte(sendDateBucket))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}

		return nil
	})
	err = db.View(func(tx *bolt.Tx) error {
		if b := tx.Bucket([]byte(chatsBucket)); b != nil {
			c := b.Cursor()
			k, v := c.First()
			for ; k != nil; k, v = c.Next() {
				id, err := strconv.ParseInt(string(k), 10, 64)
				if err != nil {
					continue
				}
				fmt.Printf("key=%s, value=%s\n", k, v)
				var newChat tb.Chat
				if err := json.Unmarshal(v, &newChat); err == nil {
					chats.chatsMap[id] = newChat
				}
			}
		}
		return nil
	})

	return
}

func sendWeather(b *tb.Bot, chatChannel chan *tb.Chat, forecast *WatherForecast) {
	for chat, ok := <-chatChannel; ok; chat, ok = <-chatChannel {

		log.Printf("Send update to %d", chat.ID)
		text := forecast.GetRudeForecast()
		message := text
		_, err := b.Send(chat, message)
		if err != nil {
			switch err.Error() {
			case "api error: Bad Request: no such user":
			default:
				fmt.Println(err)
			}
		}

	}
}

func sendUserToChan(ch chan *tb.Chat, chat *tb.Chat) {
	ch <- chat
}

func main() {

	forecast := InintWeather()

	chats := InitChats()
	b, err := tb.NewBot(tb.Settings{
		Token:  os.Getenv(tokenEnvVar),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})

	userChannel := make(chan *tb.Chat)
	go sendWeather(b, userChannel, forecast)
	go chats.SendToAllChatsDaily(b, forecast, false)

	if err != nil {
		log.Fatal(err)
		return
	}

	b.Handle("/hello", func(m *tb.Message) {
		_, err := b.Send(m.Sender, TEXT_START)
		if err != nil {
			log.Fatal(err)
		}
	})
	b.Handle("/update", func(m *tb.Message) {

		go sendUserToChan(userChannel, m.Chat)
	})

	b.Handle("/unsubscribe", func(m *tb.Message) {
		_, err := b.Send(m.Chat, fmt.Sprintf(TEXT_STOP, m.Sender.Username))
		if err != nil {
			log.Fatal(err)
		}
		chats.RemoveChat(m.Chat.ID)
		//users.RemoveUser(m.Sender.ID)

	})
	b.Handle("/stop", func(m *tb.Message) {
		_, err := b.Send(m.Chat, fmt.Sprintf(TEXT_STOP, m.Sender.Username))
		if err != nil {
			log.Fatal(err)
		}
		chats.RemoveChat(m.Chat.ID)
		//users.RemoveUser(m.Sender.ID)

	})

	b.Handle("/subscribe", func(m *tb.Message) {
		_, err := b.Send(m.Chat, fmt.Sprintf(TEXT_START, m.Sender.Username))
		if err != nil {
			log.Fatal(err)
		}
		chats.AddChat(*m.Chat)
	})

	//b.Handle("/sendall", func(m *tb.Message) {
	//		log.Println("Send to all")
	//		chats.SendToAllChats(b, 1, 1,true)
	//})

	b.Handle("/start", func(m *tb.Message) {
		_, err := b.Send(m.Chat, fmt.Sprintf(TEXT_START, m.Sender.Username))
		if err != nil {
			log.Fatal(err)
		}
		chats.AddChat(*m.Chat)
	})

	b.Handle("/info", func(m *tb.Message) {
		_, err := b.Send(m.Chat, fmt.Sprint(TEXT_INFO))
		if err != nil {
			log.Fatal(err)
		}
	})

	b.Start()
}
