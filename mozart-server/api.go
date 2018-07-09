package main

import(
  "os"
  "log"
  "net/http"
	"github.com/gorilla/mux"
	"github.com/rs/cors"
  "crypto/rand"
  "crypto/tls"
  "crypto/x509"
	"encoding/base64"
  "encoding/json"
  "fmt"
  "io/ioutil"
)

func RootHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hi there :)\n"))
}

func NodeInitialJoinHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  j := NodeInitialJoinReq{}
  json.NewDecoder(r.Body).Decode(&j)

  //Verify key
  if(j.JoinKey != config.AgentJoinKey){
    fmt.Println(config.AgentJoinKey)
    fmt.Println(j.JoinKey)
    resp := NodeJoinResp{ServerKey: "", Success: false, Error: "Invalid join key"}
    json.NewEncoder(w).Encode(resp)
    return
  }

  //Decode the CSR from base64
  csr, err := base64.URLEncoding.DecodeString(j.Csr)
  if err != nil {
      panic(err)
  }

  //Sign the CSR
  signedCert, err := signCSR(config.CaCert, config.CaKey, csr, j.AgentIp)

  //Prepare signed cert to be sent to agent
  signedCertString := base64.URLEncoding.EncodeToString(signedCert)

  //Prepare CA to be sent to agent
  ca, err := ioutil.ReadFile(config.CaCert)
  if err != nil {
    panic("cant open file")
  }
  caString := base64.URLEncoding.EncodeToString(ca)

  resp := NodeInitialJoinResp{caString, signedCertString, true, ""}
  json.NewEncoder(w).Encode(resp)
}

func NodeJoinHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  j := NodeJoinReq{}
  json.NewDecoder(r.Body).Decode(&j)

  //ADD VERIFICATION!!!!

  //Verify key
  if(j.JoinKey != config.AgentJoinKey){
    fmt.Println(config.AgentJoinKey)
    fmt.Println(j.JoinKey)
    resp := NodeJoinResp{ServerKey: "", Success: false, Error: "Invalid join key"}
    json.NewEncoder(w).Encode(resp)
    return
  }

  // //Create worker map
  // workers := make(map[string]Worker)
  //
  // //Get all workers
  // dataBytes, _ := ds.GetByPrefix("mozart/workers")
  // for k, v := range dataBytes {
  //   var data Worker
  //   err := json.Unmarshal(v, &data)
  //   if err != nil {
  //     panic(err)
  //   }
  //   workers[k] = data
  // }

  //Check if worker exist and if it has an active or maintenance status
  //if worker, ok := workers["mozart/workers/" + j.AgentIp]; ok {
  var worker Worker
  workerBytes, _ := ds.Get("mozart/workers/" + j.AgentIp)
  if workerBytes != nil {
    err := json.Unmarshal(workerBytes, &worker)
    if err != nil {
      panic(err)
    }

    if(worker.Status == "active" || worker.Status == "connected" || worker.Status == "maintenance"){
      resp := NodeJoinResp{ServerKey: "", Success: false, Error: "Host already exist and has an active or maintenance status. (This is okay if host is rejoining, just retry until it reconnects!)"}
      json.NewEncoder(w).Encode(resp)
      return
    }
  }

  //Generating key taken from http://blog.questionable.services/article/generating-secure-random-numbers-crypto-rand/
  //Generate random key
  randKey := make([]byte, 128)
  _, err := rand.Read(randKey)
  if err != nil {
    fmt.Println("Error generating a new worker key, we are going to exit here due to possible system errors.")
    os.Exit(1)
  }
  serverKey := base64.URLEncoding.EncodeToString(randKey)
  //Save key to config
  var newWorker Worker
  if workerBytes == nil {
    newWorker = Worker{AgentIp: j.AgentIp, AgentPort: "49433", ServerKey: serverKey, AgentKey: j.AgentKey, Containers: make(map[string]string), Status: "active"}
  } else {
    newWorker = Worker{AgentIp: j.AgentIp, AgentPort: "49433", ServerKey: serverKey, AgentKey: j.AgentKey, Containers: worker.Containers, Status: "active"}
  }

  //workers.mux.Lock()
  //workers.Workers[j.AgentIp] = newWorker
  //writeFile("workers", "workers.data")
  //workers.mux.Unlock()
  b, err := json.Marshal(newWorker)
  if err != nil {
    panic(err)
  }
  ds.Put("mozart/workers/" + j.AgentIp, b)

  // //Get worker container run list
  // var worker Worker
  // workerBytes, _ := ds.Get("mozart/workers/" + j.AgentIp)
  // if workerBytes != nil {
  //   err = json.Unmarshal(workerBytes, &worker)
  //   if err != nil {
  //     panic(err)
  //   }
  // }

  //Create containers map
  workerContainers := make(map[string]Container)

  //Get each container and add to map
  for _, containerName := range worker.Containers {
    var container Container
    c, _ := ds.Get("mozart/containers/" + containerName)
    err = json.Unmarshal(c, &container)
    if err != nil {
      panic(err)
    }
    workerContainers[containerName] = container
  }
  /*
  //Get containers
  dataBytes, _ = ds.GetByPrefix("mozart/containers")
  for k, v := range dataBytes {
    var data Container
    err = json.Unmarshal(v, &data)
    if err != nil {
      panic(err)
    }
    containers[k] = data
  }

  //Send containers and key to worker
  workerContainers := make(map[string]Container)
  //containers.mux.Lock()
  for _, container := range containers {
    if container.Worker == j.AgentIp {
      workerContainers[container.Name] = container
    }
  }
  */
  //containers.mux.Unlock()
  resp := NodeJoinResp{ServerKey: serverKey, Containers: workerContainers, Success: true, Error: ""}
  json.NewEncoder(w).Encode(resp)
}

func ContainersCreateHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  j := ContainerConfig{}
  json.NewDecoder(r.Body).Decode(&j)
  if(ContainersCreateVerification(j)){
    fmt.Println("Received a run request for config: ", j, "adding to queue.")
    containerQueue <- j
      /*
      err := schedulerCreateContainer(j)
      if err != nil {
        resp := Resp{false, "No workers!"} //Add better error.
        json.NewEncoder(w).Encode(resp)
        return
      }*/
      resp := Resp{true, ""}
      json.NewEncoder(w).Encode(resp)
  }else {
      resp := Resp{false, "Invalid data"} //Add better error.
      json.NewEncoder(w).Encode(resp)
  }
}

func ContainersStopHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  vars := mux.Vars(r)
  containerName := vars["container"]
  if(containerName == ""){
    resp := Resp{false, "Must provide a container name."}
    json.NewEncoder(w).Encode(resp)
    return
  }

  //Check if container exist
  //containers.mux.Lock()
  if ok, _ := ds.ifExist("mozart/containers/" + containerName); !ok {
    resp := Resp{false, "Cannot find container"}
    json.NewEncoder(w).Encode(resp)
  } else {
    resp := Resp{true, ""}
    json.NewEncoder(w).Encode(resp)
    //Add to queue
    containerQueue <- containerName
  }
  //containers.mux.Unlock()

  /*
  err := schedulerStopContainer(containerName)
  if err != nil {
    resp := Resp{false, "Cannot find container"}
    json.NewEncoder(w).Encode(resp)
    return
  }*/

}

func ContainersStateUpdateHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  type StateUpdateReq struct {
    Key string
    ContainerName string
    State string
  }

  j := StateUpdateReq{}
	json.NewDecoder(r.Body).Decode(&j)

  //TODO: Verify Worker Key here, the container must live on this host.
  //containers.mux.Lock()
  fmt.Print(j)
  var container Container
  c, _ := ds.Get("mozart/containers/" + j.ContainerName)
  err := json.Unmarshal(c, &container)
  if err != nil {
    panic(err)
  }
  if j.State == "stopped" && container.DesiredState == "stopped" {
    //delete(containers.Containers, j.ContainerName)
    ds.Del("mozart/containers/" + container.Name)
    //Update worker container run list
    var worker Worker
    workerBytes, _ := ds.Get("mozart/workers/" + container.Worker)
    err = json.Unmarshal(workerBytes, &worker)
    if err != nil {
      panic(err)
    }
    delete(worker.Containers, container.Name)
    workerToBytes, err := json.Marshal(worker)
    if err != nil {
      panic(err)
    }
    ds.Put("mozart/workers/" + container.Worker, workerToBytes)
  } else {
    //c := containers.Containers[j.ContainerName]
    container.State = j.State
    fmt.Print(container)
    //containers.Containers[j.ContainerName] = c
    b, err := json.Marshal(container)
    if err != nil {
      panic(err)
    }
    ds.Put("mozart/containers/" + container.Name, b)
  }
  //containers.mux.Unlock()

  resp := Resp{true, ""}
  json.NewEncoder(w).Encode(resp)
}

func ContainersListHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  containers := make(map[string]Container)

  //Get containers
  dataBytes, _ := ds.GetByPrefix("mozart/containers")
  for k, v := range dataBytes {
    var data Container
    err := json.Unmarshal(v, &data)
    if err != nil {
      panic(err)
    }
    containers[k] = data
  }

  resp := ContainerListResp{containers, true, ""}
  json.NewEncoder(w).Encode(resp)
}

func CheckAccountAuth(handler http.HandlerFunc) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    //Parse the form data (Required to use r.PostForm)
    err := r.ParseForm()
    if err != nil {
      resp := Resp{false, "Could not parse POST values."}
      json.NewEncoder(w).Encode(resp)
      return
    }

    //Check if Form values have been provided
    if len(r.PostForm["account"]) == 0 || len(r.PostForm["access_key"]) == 0 || len(r.PostForm["secret_key"]) == 0 {
      resp := Resp{false, "Must provide an account, access key, and secret key."}
      json.NewEncoder(w).Encode(resp)
      return
    }

    //Get account from datastore
    var account Account
    accountBytes, err := ds.Get("mozart/accounts/" + r.PostForm["account"][0])
    if accountBytes == nil {
      resp := Resp{false, "Invalid Auth. Not accounts found. (This warning is temp)"}
      json.NewEncoder(w).Encode(resp)
      return
    }
    err = json.Unmarshal(accountBytes, &account)
    if err != nil {
      resp := Resp{false, "Could not Unmarshal. Invalid Auth."}
      json.NewEncoder(w).Encode(resp)
      return
    }

    //Verify auth
    if r.PostForm["access_key"][0] != account.AccessKey || r.PostForm["secret_key"][0] != account.SecretKey {
      resp := Resp{false, "Invalid Auth."}
      json.NewEncoder(w).Encode(resp)
      return
    }

    handler(w, r)
	}
}

func AccountsCreateHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  /*type AccountCreateReq struct {
    AccessKey string
    SecretKey string
    Account Account
  }*/

  j := Account{}
	json.NewDecoder(r.Body).Decode(&j)

  if(j.Name == ""){
    resp := Resp{false, "Must provide an account name."}
    json.NewEncoder(w).Encode(resp)
    return
  }

  //Check if account exist
  if ok, _ := ds.ifExist("mozart/accounts/" + j.Name); ok {
    resp := Resp{false, "Account already exists!"}
    json.NewEncoder(w).Encode(resp)
    return
  }

  //Save account
  accountBytes, err := json.Marshal(j)
  if err != nil {
    panic(err)
  }
  ds.Put("mozart/accounts/" + j.Name, accountBytes)
  resp := Resp{true, ""}
  json.NewEncoder(w).Encode(resp)
}

func AccountsListHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  accounts := make(map[string]Account)

  //Get accounts
  dataBytes, _ := ds.GetByPrefix("mozart/accounts")
  for k, v := range dataBytes {
    var data Account
    err := json.Unmarshal(v, &data)
    if err != nil {
      panic(err)
    }
    data.AccessKey = ""
    data.SecretKey = ""
    data.Password = ""
    accounts[k] = data
  }

  resp := AccountsListResp{accounts, true, ""}
  json.NewEncoder(w).Encode(resp)
}
/*
func ContainersListWorkersHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  vars := mux.Vars(r)
  defer r.Body.Close()

  type ContainersListWorkers struct {
    Containers []Container
    Success bool
    Error string
  }

  c := ContainersListWorkers{[]Container{}, true, ""}
  for _, container := range containers.Containers {
    if (container.Worker == vars["worker"]){
      c.Containers = append(c.Containers, container)
    }
  }

  resp := c
  json.NewEncoder(w).Encode(resp)
}

func NodeListHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=UTF-8")
  w.WriteHeader(http.StatusOK)
  defer r.Body.Close()

  resp := NodeListResp{workers.Workers, true, ""}
  json.NewEncoder(w).Encode(resp)
}
*/
func startAccountAndJoinServer(serverIp string, joinPort string, caCert string, serverCert string, serverKey string){
  router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/nodes/initialjoin", NodeInitialJoinHandler)

  router.HandleFunc("/containers/create", ContainersCreateHandler)
  router.HandleFunc("/containers/stop/{container}", ContainersStopHandler)
  router.HandleFunc("/containers/list", ContainersListHandler)

  router.HandleFunc("/accounts/create", CheckAccountAuth(AccountsCreateHandler))
  router.HandleFunc("/accounts/remove", RootHandler)
  router.HandleFunc("/accounts/list", CheckAccountAuth(AccountsListHandler))

  handler := cors.Default().Handler(router)

  //Setup server config
  server := &http.Server{
        Addr: serverIp + ":" + joinPort,
        Handler: handler}

  //Start Join server
  fmt.Println("Starting join server...")
  err := server.ListenAndServeTLS(serverCert, serverKey)
  log.Fatal(err)
}

func startApiServer(serverIp string, serverPort string, caCert string, serverCert string, serverKey string) {
  router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", RootHandler)

  router.HandleFunc("/containers/create", ContainersCreateHandler)
  router.HandleFunc("/containers/stop/{container}", ContainersStopHandler)
  router.HandleFunc("/containers/list", ContainersListHandler)
  //router.HandleFunc("/containers/list/{worker}", ContainersListWorkersHandler)
  router.HandleFunc("/containers/{container}/state/update", ContainersStateUpdateHandler)
  router.HandleFunc("/containers/status/{container}", RootHandler)
  router.HandleFunc("/containers/inspect/{container}", RootHandler)

  //router.HandleFunc("/nodes/list", NodeListHandler)
  router.HandleFunc("/nodes/list/{type}", RootHandler)
  router.HandleFunc("/nodes/join", NodeJoinHandler)

  router.HandleFunc("/service/create", RootHandler)
  router.HandleFunc("/service/list", RootHandler)
  router.HandleFunc("/service/inspect", RootHandler)

  router.HandleFunc("/accounts/create", AccountsCreateHandler)
  router.HandleFunc("/accounts/remove", RootHandler)
  router.HandleFunc("/accounts/list", AccountsListHandler)

  handler := cors.Default().Handler(router)

  //Setup TLS config
  rootCa, err := ioutil.ReadFile(caCert)
  if err != nil {
    panic("cant open file")
  }
  rootCaPool := x509.NewCertPool()
  if ok := rootCaPool.AppendCertsFromPEM([]byte(rootCa)); !ok {
    panic("Cannot parse root CA.")
  }
  tlsCfg := &tls.Config{
      RootCAs: rootCaPool,
      ClientCAs: rootCaPool,
      ClientAuth: tls.RequireAndVerifyClientCert}

  //Setup server config
  server := &http.Server{
        Addr: serverIp + ":" + serverPort,
        Handler: handler,
        TLSConfig: tlsCfg}


  //Start API server
  err = server.ListenAndServeTLS(serverCert, serverKey)
	//handler := cors.Default().Handler(router)
  //err = http.ListenAndServe(ServerIp + ":" + ServerPort, handler)
  log.Fatal(err)
}
