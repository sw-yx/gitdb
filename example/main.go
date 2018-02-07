package main

import (
	"fmt"
	"time"

	db "github.com/fobilow/gitdb"
	"github.com/fobilow/gitdb/example/booking"
)

var cfg *db.Config

func init() {
	cfg = &db.Config{
		DbPath:         "./data",
		OfflineRepoDir: "./Repo/app.git",
		OnlineRemote:   "",
		OfflineRemote:  "",
		SshKey:         "",
		Factory:        Make,
		SyncInterval:   time.Minute * 5,
		EncryptionKey:  "hellobjkdkdjkdjdkjkdjooo",
	}

	db.Start(cfg)
	db.User = db.NewUser("dev", "dev@gitdb.io")

}

func main() {
	write()
	//delete()
	//search()
	// fetch()
	read()

	//db.Start(cfg)
	//db.User = db.NewUser("dev", "dev@gitdb.io")
	//db.StartGUI()
}

func write() {
	//populate model
	bm := booking.NewBookingModel()
	bm.Type = booking.Room
	bm.CheckInDate = time.Now()
	bm.CheckOutDate = time.Now()
	bm.CustomerId = "customer_1"
	bm.Guests = 2
	bm.CardsIssued = 1
	bm.NextOfKin = "Kin 1"
	bm.Status = booking.CheckedIn
	bm.UserId = "user_1"
	bm.RoomId = "room_1"

	err := db.Insert(bm)
	fmt.Println(err)

}

func read() {
	r, err := db.Get("Booking/201802/room_201802070030")
	if err != nil {
		fmt.Println(err.Error())
	} else {
		// fmt.Println(reflect.TypeOf(r))
		_, ok := r.(*booking.BookingModel)
		if ok {
			//b, _ := json.Marshal(r)
			//fmt.Println(string(b))
		}

	}
}

func delete() {
	r, err := db.Delete("Booking/201801/room_201801111823")
	if err != nil {
		fmt.Println(err.Error())
	} else {
		if r {
			fmt.Println("Deleted")
		} else {
			fmt.Println("NOT Deleted")
		}
	}
}

func search() {
	rows, err := db.Search("Booking", []string{"CustomerId"}, []string{"customer_2"}, db.SEARCH_MODE_EQUALS)
	if err != nil {
		fmt.Println(err.Error())
	} else {
		for _, r := range rows {
			fmt.Println(r)
		}
	}
}

func fetch() {
	rows, err := db.Fetch("Booking")
	if err != nil {
		fmt.Println(err.Error())
	} else {
		for _, r := range rows {
			fmt.Println(r)
		}
	}
}

func Make(modelName string) db.Model {
	var m db.Model
	switch modelName {
	case "Booking":
		return &booking.BookingModel{}
		break
	}

	return m
}
