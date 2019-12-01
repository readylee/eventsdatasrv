### eventsdatasrv

Implements an http server which retrieves, processes, and serves log events data from a remote datasource.

The datasource can be provided via the command line, by providing the hostname as an argument

#### To run the server, from the main project directory:

```bash
$ go run eventsdatasrv.go [hostname]
```

Once running, the server will print a message to STDOUT indicating its status.

With the server running, visit the following webpage: <http://localhost:8088/eventsindex> to view charts.

Currently, the local web page offers the user a couple of slider inputs for selecting the range of log events to retrieve.
Then, on demand, requests current log records within the user-specified range and renders 3d bar and pie charts based on the data.

Complete normalized data is written to eventsdata.json when the program/server run, so no need to visit the webpage to see (ALL) normalized records...

```bash
$ cat ./eventsdata.json
```

##### "Caching" of eventsdata.json #####
The data and file are updated for each request, but will not re-retrieve data from the remote source if user requests the same search twice in a row.

#### System requirements ####
1. This program requires a system installation of Go.

2. The server and webview both also assumes an internet connection, for accessing the remote data source as well as a couple of client side libraries.
