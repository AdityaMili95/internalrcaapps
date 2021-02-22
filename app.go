package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"

	cron "github.com/robfig/cron/v3"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/db"
	"google.golang.org/api/option"
)

type Cron struct {
	listenErrCh chan error
	crons       []Job
}

var (
	cronTask       *cron.Cron
	reloadCronTask *cron.Cron

	cronM *Cron

	mtx       sync.Mutex
	cronJobID = []cron.EntryID{}
)

var (
	FirebaseClient *db.Client
	actionRef      *db.Ref
)

type Job struct {
	Interval string
	Handler  func()
}

type API struct {
}

type Channel struct {
	ChannelKey string
	Footer     string
	Data       map[string]RCAData
}

type RCAData struct {
	Assignee    string
	Description string
	Environment string
	Status      int
	Title       string
	PMA         string
}

type Response struct {
	ResponseType string `json:"response_type"`
	Text         string `json:"text"`
}

type FBCred struct {
	Type          string `json:"type"`
	ProjectID     string `json:"project_id"`
	PrivatKeyID   string `json:"private_key_id"`
	PrivatKey     string `json:"private_key"`
	ClientEmail   string `json:"client_email"`
	ClientID      string `json:"client_id"`
	AuthURI       string `json:"auth_uri"`
	TokenURI      string `json:"token_uri"`
	AuthProvider  string `json:"auth_provider_x509_cert_url"`
	ClientCerturl string `json:"client_x509_cert_url"`
}

func main() {

	webserver, fbClient := initConfigAndModules()
	FirebaseClient = fbClient

	actionRef = initFBRef(fbClient)

	cronM = &Cron{
		listenErrCh: make(chan error),
	}

	RegisterCron()
	RegisterReloadCron()

	webserver.Run()
}

func RegisterReloadCron() {
	fmt.Println("REINIT RELOAD CRON")
	c := &Cron{
		listenErrCh: make(chan error),
	}

	c.register(Job{
		Interval: "25 * * * 1-5", //hourly
		Handler: func() {
			fmt.Println(nil, "JALAN DONG: ", cronJobID)
			RegisterCron()
		},
	})

	reloadCronTask = cron.New()
	c.Run(reloadCronTask)
}

func RegisterCron() {

	fmt.Println("REINIT CRON")

	var err error
	cronSchedule, err := GetAllSchedulerData()

	if err != nil {
		Println(nil, "ERROR INIT Scheduler, err: ", err)
	}

	cronM.removeAllJob()

	for channelID, interval := range cronSchedule {

		v, err := GetRCAData(channelID)
		if err != nil {
			Println(nil, "PROCESS CRON GET RCA DATA ERROR, err: ", err)
		}

		cronM.register(
			Job{
				Interval: interval,
				Handler: func() {
					msg, err := GetRCADataWrapper(channelID, 0)
					if err != nil {
						Println(nil, "PROCESS CRON ERROR, err: ", err)
						return
					}
					NotifySlack(msg, v.ChannelKey)
				},
			})
	}

	if cronTask == nil {
		cronTask = cron.New()
	} else {
		cronTask.Stop()
		cronJobID = cronM.RemoveAllRunningCron(cronJobID, cronTask)
	}

	cronM.Run(cronTask)
	Info("[!!!] Cron is Running! v1.5")
}

func (c Cron) RemoveAllRunningCron(ids []cron.EntryID, task *cron.Cron) []cron.EntryID {

	for _, entry := range ids {
		task.Remove(entry)
	}

	return []cron.EntryID{}
}

func initFBRef(client *db.Client) *db.Ref {
	actionRef := client.NewRef("ActionRequest")
	return actionRef
}

func initConfigAndModules() (*WebServer, *db.Client) {
	cfgWeb := &Option{
		Environment: "development",
		Domain:      "",
		Port:        ":4567",
	}

	webserver := NewWeb(cfgWeb)

	ctx := context.Background()
	conf := &firebase.Config{
		DatabaseURL: os.Getenv("DB_URL"),
	}

	cred := os.Getenv("FB_CRED")

	var credData FBCred
	json.Unmarshal([]byte(cred), &credData)
	file, _ := json.MarshalIndent(credData, "", " ")
	ioutil.WriteFile("./secret.json", file, 0644)

	opt := option.WithCredentialsFile("./secret.json")
	app, err := firebase.NewApp(ctx, conf, opt)

	if err != nil {
		panic(fmt.Sprintf("firebase.NewApp: %v", err))
	}
	fbClient, err := app.Database(ctx)
	if err != nil {
		panic(fmt.Sprintf("app.Firestore: %v", err))
	}

	webserver.RegisterAPI(
		API{},
	)

	return webserver, fbClient
}

func (api API) Register(router *httprouter.Router) {

	router.POST("/rca",
		api.HandleCommand,
	)

	router.GET("/rca",
		api.HandleCommand,
	)

}

func (api API) HandleCommand(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	channelID := r.FormValue("channel_id")
	command := r.FormValue("command")
	text := r.FormValue("text")
	uname := r.FormValue("user_name")
	responseUrl := r.FormValue("response_url")

	if command == "" || channelID == "" || uname == "" {
		resp := Response{
			ResponseType: "ephemeral",
			Text:         fmt.Sprintf("[Custom Binary] Invalid Param, command: %s, channelID: %s, uname: %s, request: %+v\n", command, channelID, uname, r),
		}

		Printf(nil, "[Custom Binary] Invalid Param, command: %s, channelID: %s, uname: %s, request: %+v\n", command, channelID, uname, r)
		WriteResponse(w, resp)
		return
	}

	var err error
	msg := ""

	tempSlackMsg := SlackMsgStructure{}
	shouldSlackNotify := false

	if command == "/listrca" {
		tempSlackMsg, err = GetRCADataWrapper(channelID, 0)
		msg = fmt.Sprintf("RCA List requested by %s", uname)
		shouldSlackNotify = true
	} else if command == "/listdonerca" {
		tempSlackMsg, err = GetRCADataWrapper(channelID, 1)
		msg = fmt.Sprintf("RCA List requested by %s", uname)
		shouldSlackNotify = true
	} else if command == "/addrca" {
		msg, err = AddRCA(uname, text, channelID)
	} else if command == "/donerca" {
		msg, err = DoneRCA(uname, text, channelID, 1)
	} else if command == "/removerca" {
		msg, err = DoneRCA(uname, text, channelID, 3)
	} else if command == "/doneallrca" {
		msg, err = DoneAllRCA(uname, channelID)
	} else if command == "/setscheduler" {
		msg, err = SetScheduler(uname, channelID, text)
	} else if command == "/removescheduler" {
		msg, err = SetScheduler(uname, channelID, "")
	} else if command == "/setslackwebhook" {
		msg, err = SetWebhook(uname, channelID, text)
	} else if command == "/setfooter" {
		msg, err = SetFooter(uname, channelID, text)
	} else if command == "/setpma" {
		msg, err = SetPMA(uname, channelID, text)
	} else if command == "/internalrcahelp" {
		msg = HelpRCA()
	}

	if err != nil {
		resp := Response{
			ResponseType: "ephemeral",
			Text:         err.Error(),
		}

		WriteResponse(w, resp)
		return
	}

	if shouldSlackNotify {
		NotifySlack(tempSlackMsg, responseUrl)
	}

	tempSlackResp := SlackMsgStructure{}
	tempblock := GetSlackMessageStructure(msg)
	tempSlackResp.Blocks = append(tempSlackResp.Blocks, tempblock)

	WriteResponse(w, tempSlackResp)

}

func WriteResponse(w http.ResponseWriter, response interface{}) {
	b, _ := json.Marshal(response)

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(200)
	w.Write(b)
}

func NotifySlack(message SlackMsgStructure, channelKey string) error {

	slackM := NewSlackModule(channelKey, "Production")
	slackM.PublishSlack(message)
	return nil
}

func HelpRCA() string {
	return "*Internal RCA BOT Command Help*\n\n• `/listrca` - Get List Active RCA :memo::memo:\n• `/listdonerca` - Get list of Done RCA :\n• `/addrca - Title Desc Assignee [PMATicketURL] [Staging|Production]` - Add New RCA\n• `/removerca issueID` - Remove RCA\n• `/donerca issueID` - Set RCA to Done\n• `/doneallrca` - *Done all* active RCA :warning::warning:\n• `/setpma issueID PMATicketURL` - Set PMA Ticket for issue\n• `/setscheduler schedule` (*<https://pkg.go.dev/github.com/robfig/cron/v3|format>*) - Set Scheduler for RCA List\n• `/setslackwebhook` - Set slack webhook for scheduler (*for the webhook url*, contact: <@U75J4HEF9>)\n• `/removescheduler` - Remove Scheduler for RCA List \n• `/setfooter` - Set *Custom* footer notes that shown at the bottom of RCA List"
}

func SetScheduler(uname, channelID, text string) (string, error) {
	ctx := context.Background()

	v, err := GetRCAData(channelID)
	if err != nil {
		return "", err
	}

	if v.ChannelKey == "" {
		return "", errors.New("Channel Webhook not set, set using command /setslackwebhook [webhook_key]")
	}

	updateTxn := func(node db.TransactionNode) (interface{}, error) {
		return text, nil
	}

	action := "removed"

	if text != "" {
		action = fmt.Sprintf("set to %s", text)
	}

	msg := fmt.Sprintf("RCA List scheduler %s by %s", action, uname)
	err = FirebaseClient.NewRef(fmt.Sprintf("Scheduler/%s", channelID)).Transaction(ctx, updateTxn)

	if err == nil {
		RegisterCron()
	}

	return msg, err

}

func SetFooter(uname, channelID, text string) (string, error) {
	ctx := context.Background()

	updateTxn := func(node db.TransactionNode) (interface{}, error) {
		return text, nil
	}

	return fmt.Sprintf("Message RCA - updated to %s by %s", strings.Replace(text, "\n", "", -1), uname), FirebaseClient.NewRef(fmt.Sprintf("Channel/%s/Footer", channelID)).Transaction(ctx, updateTxn)
}

func SetWebhook(uname, channelID, text string) (string, error) {
	ctx := context.Background()

	updateTxn := func(node db.TransactionNode) (interface{}, error) {
		return text, nil
	}

	return fmt.Sprintf("Slack Webook set by %s", uname), FirebaseClient.NewRef(fmt.Sprintf("Channel/%s/channelKey", channelID)).Transaction(ctx, updateTxn)
}

func DoneAllRCA(uname, channelID string) (string, error) {

	v, err := GetRCAData(channelID)
	if err != nil {
		return "", err
	}

	for issueID, _ := range v.Data {
		err = SetDoneRCAData(channelID, issueID, 1)
		if err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("All RCA Set to Done by %s", uname), nil
}

func DoneRCA(uname, text, channelID string, status int) (string, error) {
	desc := strings.Split(text, " ")

	if len(desc) < 1 || desc[0] == "" {
		return "", errors.New("Command invalid")
	}

	setTo := "Set to Done"
	if status == 3 {
		setTo = "Removed"
	}

	var v RCAData
	ctx := context.Background()
	err := FirebaseClient.NewRef(fmt.Sprintf("Channel/%s/data/%s", channelID, desc[0])).Get(ctx, &v)

	if err != nil {
		return "", err
	}

	if v.Title == "" {
		return "", errors.New(fmt.Sprintf("FAILED - Invalid RCA ID (%s) - Action: %s by %s", desc[0], setTo, uname))
	}

	return fmt.Sprintf("RCA %s (`%s`) %s by %s", v.Title, desc[0], setTo, uname), SetDoneRCAData(channelID, desc[0], status)
}

func SetPMA(uname, text, channelID string) (string, error) {
	desc := strings.Split(text, " ")

	if len(desc) < 2 || desc[0] == "" || desc[1] == "" {
		return "", errors.New("Command invalid")
	}

	var v RCAData
	ctx := context.Background()
	err := FirebaseClient.NewRef(fmt.Sprintf("Channel/%s/data/%s", channelID, desc[0])).Get(ctx, &v)

	if err != nil {
		return "", err
	}

	if v.Title == "" {
		return "", errors.New(fmt.Sprintf("FAILED - Invalid RCA ID (%s) - Action: Set PMA Ticket by %s", desc[0], uname))
	}

	updateTxn := func(node db.TransactionNode) (interface{}, error) {
		return desc[1], nil
	}

	return fmt.Sprintf("RCA %s (`%s`) PMA Ticket set by %s", v.Title, desc[0], uname), FirebaseClient.NewRef(fmt.Sprintf("Channel/%s/data/%s/PMA", channelID, desc[0])).Transaction(ctx, updateTxn)
}

func SetDoneRCAData(channelID, issueID string, status int) error {
	ctx := context.Background()

	updateTxn := func(node db.TransactionNode) (interface{}, error) {
		return status, nil
	}

	return FirebaseClient.NewRef(fmt.Sprintf("Channel/%s/data/%s/Status", channelID, issueID)).Transaction(ctx, updateTxn)
}

func AddRCA(uname, text, channelID string) (string, error) {

	desc := strings.Split(text, " ")

	if len(desc) < 3 {
		return "", errors.New("Command invalid")
	}

	pma := "" //optional
	if len(desc) > 3 {
		pma = desc[3]
	}

	env := "Production" //optional
	if len(desc) > 4 {
		env = strings.Title(desc[4])
	}

	data := RCAData{
		Assignee:    desc[2],
		Description: desc[1],
		Environment: env,
		Status:      0,
		Title:       desc[0],
		PMA:         pma,
	}

	ctx := context.Background()
	if err := FirebaseClient.NewRef(fmt.Sprintf("Channel/%s/data/%s", channelID, uuid.New())).Set(ctx, data); err != nil {
		return "", err
	}

	return fmt.Sprintf("RCA %s Added by %s", desc[0], uname), nil
}

func CaptureCronPanic(handler func()) func() {
	return func() {
		defer func() {
			var err error
			r := recover()
			if r != nil {
				switch t := r.(type) {
				case string:
					err = errors.New(t)
				case error:
					err = t
				default:
					err = errors.New("unknown error")
				}
				Println(nil, "[Cron Process Got Panic] ", err.Error())
				panic(fmt.Sprintf("%s, Stack: %+v", "Cron Process Got Panic!!!", err.Error()))
			}
		}()
		handler()
	}
}

func (c *Cron) removeAllJob() {
	c.crons = []Job{}
}

func (c *Cron) register(j Job) {
	j.Handler = CaptureCronPanic(j.Handler)
	c.crons = append(c.crons, j)
}

func (c *Cron) ListenError() <-chan error {
	return c.listenErrCh
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (c *Cron) Run(taskCron *cron.Cron) {

	if len(c.crons) == 0 {
		Println(nil, "[!!!] Cron is Empty!")
		return
	}

	for _, j := range c.crons {

		//fnameTemp := strings.Split(getFunctionName(j.Handler), "/")
		//fname := fnameTemp[len(fnameTemp)-1]
		id, err := taskCron.AddFunc(j.Interval, j.Handler)

		mtx.Lock()
		cronJobID = append(cronJobID, id)
		mtx.Unlock()

		if err != nil {
			Println(nil, "Assign Job ERROR: ", err)
		}
	}

	taskCron.Start()
}

func GetRCADataWrapper(channelID string, getStatus int) (SlackMsgStructure, error) {
	v, err := GetRCAData(channelID)

	if err != nil {
		return SlackMsgStructure{}, err
	}

	return ConstructRCADataString(v, getStatus), nil
}

func GetRCAData(channelID string) (Channel, error) {
	var v Channel
	ctx := context.Background()
	err := FirebaseClient.NewRef(fmt.Sprintf("Channel/%s", channelID)).Get(ctx, &v)
	return v, err
}

func ConstructRCADataString(v Channel, getStatus int) SlackMsgStructure {
	title := fmt.Sprintf("*Internal Sharing & RCA List*\n\n")

	staging := ""
	production := ""

	emot := "bangbang"

	if getStatus == 1 {
		title = fmt.Sprintf("*Internal Sharing & RCA List - DONE*\n\n")
		emot = "white_check_mark"
	}

	for issueID, is := range v.Data {
		if is.Status != getStatus {
			continue
		}

		pma := ""

		if is.PMA != "" {
			pma = fmt.Sprintf("- <%s|*PMA*> ", is.PMA)
		}

		if is.Environment == "Staging" {
			staging += fmt.Sprintf(">\t:%s:  *%s* %s\n\t\t\t• `Issue ID:` %s\n\t\t\t• `Description:` %s\n\t\t\t• `Assignee:` %s\n\n", emot, is.Title, pma, issueID, is.Description, is.Assignee)
		} else {
			production += fmt.Sprintf(">\t:%s:  *%s* %s\n>\t\t\t• `Issue ID:` %s\n>\t\t\t• `Description:` %s\n>\t\t\t• `Assignee:` %s\n\n", emot, is.Title, pma, issueID, is.Description, is.Assignee)
		}
	}

	slackMsg := SlackMsgStructure{}
	blockTitle := GetSlackMessageStructure(title)
	slackMsg.Blocks = append(slackMsg.Blocks, blockTitle)

	anyRCA := false

	if staging != "" {
		anyRCA = true
		block := GetSlackMessageStructure(":arrow_right: `Environment: Staging`\n\n" + staging)
		slackMsg.Blocks = append(slackMsg.Blocks, GetSlackDividerBlock())
		slackMsg.Blocks = append(slackMsg.Blocks, block)

	}

	if production != "" {
		anyRCA = true
		spacing := ""
		if staging != "" {
			spacing += "\n\n"
		}

		block := GetSlackMessageStructure(spacing + ":arrow_right: `Environment: Production`\n\n" + production)
		slackMsg.Blocks = append(slackMsg.Blocks, GetSlackDividerBlock())
		slackMsg.Blocks = append(slackMsg.Blocks, block)
	}

	if !anyRCA || getStatus == 1 {

		foot := ""
		if !anyRCA && getStatus == 0 {
			foot = "\n\n*No RCA Item - Great Job Team* ! :muscle: :muscle: :muscle:"
		}

		quote := "_`True stability results when presumed order and presumed disorder are balanced. A truly stable system expects the unexpected, is prepared to be disrupted, waits to be transformed`_ - Tom Robbins\n"

		slackMsg.Blocks = append(slackMsg.Blocks, GetSlackDividerBlock())
		block := GetSlackMessageStructure(quote)
		slackMsg.Blocks = append(slackMsg.Blocks, block)

		if foot != "" {
			slackMsg.Blocks = append(slackMsg.Blocks, GetSlackDividerBlock())
			blockFoot := GetSlackMessageStructure(foot)
			slackMsg.Blocks = append(slackMsg.Blocks, blockFoot)

		}

	} else {

		foot := "\n`Lets maintain our stability together with #gotongroyong and #makeithappenmakeitbetter spirit` :muscle: \n\n_*Please prepare the Deck* ya Team!"

		if v.Footer != "" {
			foot = v.Footer
		}

		slackMsg.Blocks = append(slackMsg.Blocks, GetSlackDividerBlock())
		blockFoot := GetSlackMessageStructure(foot)
		slackMsg.Blocks = append(slackMsg.Blocks, blockFoot)
	}

	slackMsg = AppendFootNotes(slackMsg)

	return slackMsg
}

func GetSlackMessageStructure(msg string) BlockStructure {
	return BlockStructure{
		Type: "section",
		Text: &BlockText{
			Type: "mrkdwn",
			Text: msg,
		},
	}
}

func GetSlackDividerBlock() BlockStructure {
	return BlockStructure{
		Type: "divider",
	}
}

func AppendFootNotes(slackMsg SlackMsgStructure) SlackMsgStructure {
	slackMsg.Blocks = append(slackMsg.Blocks, GetSlackDividerBlock())
	slackMsg.Blocks = append(slackMsg.Blocks, GetSlackMessageStructure("\nType `/internalrcahelp` :dart: for more commands"))
	return slackMsg
}

func GetAllRCAData() (map[string]Channel, error) {
	var channels map[string]Channel

	ctx := context.Background()
	err := FirebaseClient.NewRef("Channel").Get(ctx, &channels)
	return channels, err
}

func GetAllSchedulerData() (map[string]string, error) {
	var scheduler map[string]string
	ctx := context.Background()
	err := FirebaseClient.NewRef("Scheduler").Get(ctx, &scheduler)
	return scheduler, err
}

//global
func startProcessCron() {
	channels, err := GetAllRCAData()

	if err != nil {
		Println(nil, err)
	}

	for _, v := range channels {
		slackMsg := ConstructRCADataString(v, 0)
		NotifySlack(slackMsg, v.ChannelKey)
	}
}
