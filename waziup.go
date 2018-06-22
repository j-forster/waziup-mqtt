package waziup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/j-forster/mqtt"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	// "net/http"
	//  _ "net/http/pprof"
)

var (
	AuthUnavailable = errors.New("Auth Server unavailable.")
	Forbidden       = errors.New("Forbidden.")
	UnknownErr      = errors.New("Auth error.")
)

////////////////////////////////////////////////////////////////////////////////

var API = os.Getenv("WAZIUP_API")
var MONGODB = os.Getenv("MONGODB")

type Permission struct {
	Resource string   `json:"resource"`
	Scopes   []string `json:"scopes"`
}

func (p *Permission) isSensor() bool {
	return p.Resource != "Socials" &&
		p.Resource != "History" &&
		p.Resource != "Domains" &&
		p.Resource != "Notifications" &&
		p.Resource != "Sensors"
}

func (p *Permission) canView() bool {
	for _, scope := range p.Scopes {
		if scope == "sensors:view" {
			return true
		}
	}
	return false
}

func (p *Permission) canUpdate() bool {
	for _, scope := range p.Scopes {
		if scope == "sensors:update" {
			return true
		}
	}
	return false
}

type SensorPermission struct {
	View, Update bool
}

type Sensors map[string]SensorPermission

func MapSensors(perms []Permission) Sensors {
	sensors := make(Sensors)
	for _, perm := range perms {
		if perm.isSensor() {
			sensors[perm.Resource] = SensorPermission{perm.canView(), perm.canUpdate()}
		}
	}
	return sensors
}

////////////////////////////////////////////////////////////////////////////////

type WaziupHandler struct {
	coll *mongo.Collection
}

func NewWaziupHandler() *WaziupHandler {

	client, err := mongo.NewClient("mongodb://" + MONGODB + ":27017")
	if err != nil {
		log.Fatal(err)
	}

	err = client.Connect(context.TODO())
	if err != nil {
		log.Fatal(err)
	}

	collection := client.Database("waziup_history").Collection("waziup-waziup")

	log.Print("Connected to MongoDB.")

	return &WaziupHandler{coll: collection}
}

func (h *WaziupHandler) Connect(ctx *mqtt.Context, username, password string) error {

	data := fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, password)
	resp, err := http.Post("http://"+API+"/api/v1/auth/token", "application/json", strings.NewReader(data))
	if err != nil {
		log.Printf("%v Connected: Auth Server Unavailable: %v", ctx.ClientID, err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("%v Connected: Auth Server: %v", ctx.ClientID, resp.Status)
		return AuthUnavailable
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v Connected: Auth Server: %v", ctx.ClientID, err)
		return UnknownErr
	}
	token := string(body)

	//////////////////////////////////////////////////////////////////////////////

	client := http.Client{}

	req, _ := http.NewRequest("GET", "http://"+API+"/api/v1/auth/permissions", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	resp, err = client.Do(req)

	if err != nil {
		log.Printf("%v Connected: Auth Server Unavailable: %v", ctx.ClientID, err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("%v Connected: Auth Server: %v", ctx.ClientID, resp.Status)
		return AuthUnavailable
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v Connected: Auth Server: %v", ctx.ClientID, err)
		return UnknownErr
	}

	var permissions []Permission
	err = json.Unmarshal(body, &permissions)
	if err != nil {
		log.Printf("%v Connected: Auth Server: %v", ctx.ClientID, err)
		return UnknownErr
	}

	sensors := MapSensors(permissions)

	ctx.Set("waziup-sensors", sensors)

	//////////////////////////////////////////////////////////////////////////////

	log.Printf("%v Connected: '%v'", ctx.ClientID, username)

	log.Printf("%v Sensor Perm: %v", ctx.ClientID, sensors)

	// log.Printf("Auth: %v", string(body))

	return nil // no error == accept everyone
}

func (h *WaziupHandler) Disconnect(ctx *mqtt.Context) {

	log.Printf("%v Disconnected", ctx.ClientID)
}

func (h *WaziupHandler) Publish(ctx *mqtt.Context, msg *mqtt.Message) error {

	sensors := ctx.Get("waziup-sensors").(Sensors)
	path := strings.Split(msg.Topic, "/")
	name := path[0]
	perm, ok := sensors[name]
	log.Printf("%v %v %v %v", path, perm, ok, sensors)
	if !ok || !perm.Update {
		log.Printf("%v Publish FORBIDDEN: '%v' [%v] ", ctx.ClientID, msg.Topic, len(msg.Buf))
		return Forbidden
	}

	// log.Printf("%v Publish: '%v' [%v]", ctx.ClientID, msg.Topic, len(msg.Buf))

	if len(path) == 2 {

		var EC bson.ElementConstructor

		doc := bson.NewDocument(
			EC.String("entityID", name),
			EC.String("entityType", "SensingDevice"),
			EC.String("attributeID", path[1]),
			EC.Time("timestamp", time.Now()))

		var data interface{}
		if err := json.Unmarshal(msg.Buf, &data); err == nil {
			doc.Append(EC.Interface("value", data))
		} else {
			doc.Append(EC.Binary("value", msg.Buf))
		}

		h.coll.InsertOne(context.Background(), doc)
	}
	return nil
}

func (h *WaziupHandler) Subscribe(ctx *mqtt.Context, topic string, qos byte) error {

	sensors := ctx.Get("waziup-sensors").(Sensors)
	name := strings.Split(topic, "/")[0]

	perm, ok := sensors[name]
	if !ok || !perm.View {
		log.Printf("%v Subscribe FORBIDDEN: '%v'", ctx.ClientID, topic)
		return Forbidden
	}

	log.Printf("%v Subscribe: '%v'", ctx.ClientID, topic)

	return nil
}

/*
func main() {

	handler := NewWaziupHandler()
	log.Println("Up and running: Port 1883")
	mqtt.ListenAndServe(":1883", handler)
}
*/
