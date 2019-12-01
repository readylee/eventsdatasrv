package main

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "net/url"
    "os"
    str "strings"
    "time"
)

// Constants

// local filenames
const CHART_HTML string ="html/chartview.html"
const EVENTS_LOCAL_JSON_FILE_NAME string = "eventsdata.json"
const EVENTS_META_DATA_FILE_NAME string = "eventsmetadata.json"

const LOCAL_SERVER_PORT string = ":8088"

const DEFAULT_RANGE_START string = "0"
const TIMEOUT_SECONDS time.Duration = 20

const LOGON_SUCCESS string = "Logon-Success"
const LOGON_FAILURE string = "Logon-Failure"

// default data source and its endpoints
const REMOTE_DATA_PROTOCOL string = "https://"

const AUTH_ENDPOINT string = "/auth"
const EVENTS_ENDPOINT string = "/get-events"

var remoteDataUrl = ""

type RawLogEventRecord struct {
    AcmeApiId     int      `json:"id"`
    UserName      string   `json:"user_Name"`
    SourceIp      []string   `json:"ips"`
    Target        string   `json:"target"`
    Action        string   `json:"EVENT_0_ACTION"`
    EventTime     int      `json:"DateTimeAndStuff"`
}

type NormalizedLogEventRecord struct {
    AcmeApiId     int
    UserName      string
    SourceIp      string
    Target        string
    Action        string
    EventTime     int
}

type EventRecordSetMeta struct {
    EntryCount    int       `json:"EntryCount"`
    LastEntryHash string    `json:"LastEntryHash"`
}

// Instantiate a (local) server to receive requests for data.
// The server handles a few endpoints for retrieving/processing
// data from remoteDataUrl, then serving it
func main() {

    ensureRemoteDataSource(os.Args)

    auth, dataSource := attemptAuthenticateToDataServer()
    if dataSource == nil {
        os.Exit(1)
    }

    runLocalServer(auth, dataSource)
}

func handleError(e error) {
    if e != nil {
        panic(e)
    }
}

func ensureRemoteDataSource(args []string) {

    // define the remote datasource, from cmd line if provided, yet enforcing our protocol
    remoteDataUrl = "duoauth.me"
    remoteDataArg := ""
    if (len(args) > 1) {
        remoteDataArg = args[1]
    }
    if (remoteDataArg != "") {
        remoteDataUrl = remoteDataArg
    }

    if remoteDataUrl == "" {
        // bail if for some reason we don't have a data src at this point
        os.Exit(1)
    }

    remoteDataUrl = concatStrings([]string{REMOTE_DATA_PROTOCOL, remoteDataUrl})
}

func attemptAuthenticateToDataServer() (string, *http.Client) {

    // basic HTTP client
    dataSource := &http.Client{
        Timeout: time.Second * TIMEOUT_SECONDS,
    }

    // "Authenticate" then initResp.Body should be an auth token
    initResp, initErr := dataSource.Get(concatStrings([]string{remoteDataUrl, AUTH_ENDPOINT}))

    handleError(initErr)

    defer initResp.Body.Close()

    if initResp.StatusCode == http.StatusOK {
        // read initResp.Body into respBytes
        respBytes, readErr := ioutil.ReadAll(initResp.Body)

        handleError(readErr)

        auth := string(respBytes)
        auth = str.TrimSuffix(auth, "\n")

        return auth, dataSource
    }

    return "", nil
}

// Define a couple of endpoints to serve from localhost:8088
// Start the server
func runLocalServer(authString string, dataClient *http.Client) {

    // why not just go ahead and get all the events...
    lastSearch := "?from=0"
    attemptGetSomeEvents(authString, dataClient, lastSearch)

    eventsDataHandler := func(wr http.ResponseWriter, req *http.Request) {
        // serve the local json, which should be at EVENTS_LOCAL_JSON_FILE_NAME
        if (authString != "") {
            parsedParams := parseEventsRequestParams(req.URL.Query())
            if (lastSearch != parsedParams) {
                attemptGetSomeEvents(authString, dataClient, parsedParams)
                lastSearch = parsedParams
            }
            http.ServeFile(wr, req, EVENTS_LOCAL_JSON_FILE_NAME)
        }
    }
    http.HandleFunc("/eventsdata", eventsDataHandler)

    eventsMetaHandler := func(wr http.ResponseWriter, req *http.Request) {
        // serve the html at EVENTS_META_DATA_FILE_NAME
        if (authString != "") {
            attemptGetEventsMeta(authString, dataClient)
            http.ServeFile(wr, req, EVENTS_META_DATA_FILE_NAME)
        }
    }
    http.HandleFunc("/eventsmetadata", eventsMetaHandler)

    indexHandler := func(wr http.ResponseWriter, req *http.Request) {
        // serve the html at CHART_HTML
        http.ServeFile(wr, req, CHART_HTML)
    }
    http.HandleFunc("/eventsindex", indexHandler)

    fmt.Println(fmt.Sprintf("Now running processing server... To view charts, visit http://localhost%s/eventsindex", LOCAL_SERVER_PORT))
    log.Fatal(http.ListenAndServe(LOCAL_SERVER_PORT, nil))
}

// Parse some http.Request params
// @param queryParamsObj(url.Values) - the params to parse
// @returns paramString(string) - a request param string
func parseEventsRequestParams(queryParamsObj url.Values) string {
    paramString := ""
    rangeStartVal := "0"

    fromQuery := queryParamsObj["from"]
    toQuery := queryParamsObj["to"]

    if (len(fromQuery) > 0 && fromQuery[0] != "") {
        rangeStartVal = fromQuery[0]
    }
    paramString = fmt.Sprintf("?from=%s", rangeStartVal)

    if (len(toQuery) > 0 && toQuery[0] != "") {
        rangeEndQuery := fmt.Sprintf("&to=%s", toQuery[0])
        paramString = concatStrings([]string{paramString, rangeEndQuery})
    }
    return paramString
}

// Construct a string from substring parts
// @param fragments([]string) - parts to concatenate into
// @returns res(string) - the complete string
func concatStrings(fragments []string) string {
    res := ""
    for _, fragment := range fragments {
       res = str.Join([]string{res, fragment}, "")
    }
    return res
}

func constructAuthorizedRequest(token string, url string, httpmethod string) (*http.Request, error) {
   r, e := http.NewRequest(httpmethod, url, nil)
   r.Header.Add("Authorization", token)
   return r, e
}

func getRemoteRequestBody(authorization string, reqUrl string, client *http.Client) (string) {
   req, err := constructAuthorizedRequest(authorization, reqUrl, "GET")
   handleError(err)

   resp, respErr := client.Do(req)
   handleError(respErr)
   defer resp.Body.Close()

   respBytes, ioErr := ioutil.ReadAll(resp.Body)
   handleError(ioErr)

   return string(respBytes)
}

// Retrieves current event records metadata from remote data source and saves locally
// @param authToken(string) = authentication token
// @param c(*http.Client) = the remote data source request runner
func attemptGetEventsMeta(authToken string, c *http.Client) {
   requestUrl := concatStrings([]string{remoteDataUrl, EVENTS_ENDPOINT})
   content := getRemoteRequestBody(authToken, requestUrl, c)

   var eventsMetaData EventRecordSetMeta    
   json.Unmarshal([]byte(content), &eventsMetaData)

   saveJSONDataLocally(eventsMetaData, EVENTS_META_DATA_FILE_NAME)
}

// Perform an api call to get some subset of the events record set
// @param authToken(string) - the auth token string to send with api requests headers
// @param c(http.Client) - the http client
// @param params(string) - a query string for querying records from our remote data source
// Eventually stores/writes normalized events records to EVENTS_LOCAL_JSON_FILE
func attemptGetSomeEvents(authToken string, c *http.Client, params string) {
    requestUrl := concatStrings([]string{remoteDataUrl, EVENTS_ENDPOINT, params})

    allEvents := getRemoteRequestBody(authToken, requestUrl, c)

    var logEventRecordsCollection []RawLogEventRecord
    json.Unmarshal([]byte(allEvents), &logEventRecordsCollection)

    normalizeLogRecordsAndStoreLocally(logEventRecordsCollection )
}

func saveJSONDataLocally(data interface{}, filename string) {
     jsonData, _ := json.MarshalIndent(data, "", " ")
    _ = ioutil.WriteFile(filename, jsonData, 0644)
}


// Normalize the raw event logs data
// Store the data in JSON format locally at EVENTS_LOCAL_JSON_FILE_NAME
// @param logEvents([]RawLogEventRecord) = a collection of raw, unnormalized log records
func normalizeLogRecordsAndStoreLocally(logEvents []RawLogEventRecord) {
    var normalizedRecordsCollection []NormalizedLogEventRecord

    // loop through log events, normalize each...
    for i := 0; i < len(logEvents); i++ {
        normalizedEvent := getNormalizedLogEvent(logEvents[i])
        normalizedRecordsCollection = append(normalizedRecordsCollection, normalizedEvent)
    }
    saveJSONDataLocally(normalizedRecordsCollection, EVENTS_LOCAL_JSON_FILE_NAME)
}

// Create a normalized log event record from raw record data
// @param rawLogEvent(RawLogEventRecord) = a raw log event record
// @returns NormalizedLogEventRecord = a normalized record
func getNormalizedLogEvent(rawLogEvent RawLogEventRecord) NormalizedLogEventRecord {
    // create normalized record from raw record
    // return the normalized record
    acmeId := rawLogEvent.AcmeApiId
    userName := getUsername(rawLogEvent.UserName)
    sourceIp := rawLogEvent.SourceIp[0]
    targetUrl := rawLogEvent.Target
    logAction := mapLogActionResponse(rawLogEvent.Action)
    eventTime := rawLogEvent.EventTime

    return NormalizedLogEventRecord{acmeId, userName, sourceIp, targetUrl, logAction, eventTime}
}

// In the raw, not-yet-normalized records, the Username value 
// in the Username field might be prefixed w/ "Username is:" or have capitals
// @param rawUserField(string) = a possibly prefixed username field value
// @returns string = just the username, trim and properly lowercased
func getUsername(rawUserField string) string {
    if rawUserField == "" {
        return rawUserField
    }

    s := str.ToLower(str.TrimSpace(rawUserField))
    spl := str.Split(s, ":")

    switch {
    case len(spl) > 1:
        return str.TrimSpace(spl[1])
    default:
        return s
    }
}

func mapLogActionResponse(loginString string) string {
    loginStringLowercase := str.ToLower(loginString)

    switch {
    case str.Contains(loginStringLowercase, "success"):
        return LOGON_SUCCESS
    case str.Contains(loginStringLowercase, "fail"):
        return LOGON_FAILURE
    default:
        return fmt.Sprintf("Logon-%s", loginString)
    }    
}
