package debugapi

import (
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/FavorLabs/favorX/pkg/settlement/traffic"
	"net/http"
)

func (s *Service) trafficInit(w http.ResponseWriter, r *http.Request) {
	err := s.traffic.TrafficInit()
	if err != nil {
		s.logger.Error("debugApi-TrafficInit call failed: %v", err)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.OK(w, nil)
}

func (s *Service) MustRegisterTraffic(traffic traffic.ApiInterface) {
	s.traffic = traffic
}
