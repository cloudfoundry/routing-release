package handlers

import (
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	uaaclient "code.cloudfoundry.org/uaa-go-client"
)

type TcpRouteMappingsHandler struct {
	uaaClient uaaclient.Client
	validator RouteValidator
	db        db.DB
	logger    lager.Logger
	maxTTL    int
}

func NewTcpRouteMappingsHandler(uaaClient uaaclient.Client, validator RouteValidator, database db.DB, ttl int, logger lager.Logger) *TcpRouteMappingsHandler {
	return &TcpRouteMappingsHandler{
		uaaClient: uaaClient,
		validator: validator,
		db:        database,
		logger:    logger,
		maxTTL:    ttl,
	}
}

func (h *TcpRouteMappingsHandler) List(w http.ResponseWriter, req *http.Request) {
	log := h.logger.Session("list-tcp-route-mappings")

	err := h.uaaClient.DecodeToken(req.Header.Get("Authorization"), RoutingRoutesReadScope)
	if err != nil {
		handleUnauthorizedError(w, err, log)
		return
	}
	query := req.URL.Query()
	var routes []models.TcpRouteMapping
	if len(query["isolation_segment"]) > 0 {
		routes, err = h.db.ReadFilteredTcpRouteMappings("isolation_segment", query["isolation_segment"])
	} else {
		routes, err = h.db.ReadTcpRouteMappings()
	}
	if err != nil {
		handleDBCommunicationError(w, err, log)
		return
	}
	encoder := json.NewEncoder(w)
	err = encoder.Encode(routes)
	if err != nil {
		handleProcessRequestError(w, err, log)
	}
}

func (h *TcpRouteMappingsHandler) Upsert(w http.ResponseWriter, req *http.Request) {
	log := h.logger.Session("create-tcp-route-mappings")

	err := h.uaaClient.DecodeToken(req.Header.Get("Authorization"), RoutingRoutesWriteScope)
	if err != nil {
		handleUnauthorizedError(w, err, log)
		return
	}

	decoder := json.NewDecoder(req.Body)
	var tcpMappings []models.TcpRouteMapping
	err = decoder.Decode(&tcpMappings)
	if err != nil {
		handleProcessRequestError(w, err, log)
		return
	}

	// set defaults
	for i := 0; i < len(tcpMappings); i++ {
		tcpMappings[i].SetDefaults(h.maxTTL)
	}

	log.Info("request", lager.Data{"tcp_mapping_creation": tcpMappings})

	// fetch current router groups
	routerGroups, err := h.db.ReadRouterGroups()
	if err != nil {
		handleDBCommunicationError(w, err, log)
		return
	}

	apiErr := h.validator.ValidateCreateTcpRouteMapping(tcpMappings, routerGroups, h.maxTTL)
	if apiErr != nil {
		handleProcessRequestError(w, apiErr, log)
		return
	}

	for _, tcpMapping := range tcpMappings {
		err = h.db.SaveTcpRouteMapping(tcpMapping)
		if err != nil {
			handleDBCommunicationError(w, err, log)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *TcpRouteMappingsHandler) Delete(w http.ResponseWriter, req *http.Request) {
	log := h.logger.Session("delete-tcp-route-mappings")

	err := h.uaaClient.DecodeToken(req.Header.Get("Authorization"), RoutingRoutesWriteScope)
	if err != nil {
		handleUnauthorizedError(w, err, log)
		return
	}
	decoder := json.NewDecoder(req.Body)

	var tcpMappings []models.TcpRouteMapping
	err = decoder.Decode(&tcpMappings)
	if err != nil {
		handleProcessRequestError(w, err, log)
		return
	}

	log.Info("request", lager.Data{"tcp_mapping_deletion": tcpMappings})

	apiErr := h.validator.ValidateDeleteTcpRouteMapping(tcpMappings)
	if apiErr != nil {
		handleProcessRequestError(w, apiErr, log)
		return
	}

	for _, tcpMapping := range tcpMappings {
		err = h.db.DeleteTcpRouteMapping(tcpMapping)
		if err != nil {
			if dberr, ok := err.(db.DBError); !ok || dberr.Type != db.KeyNotFound {
				handleDBCommunicationError(w, err, log)
				return
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
