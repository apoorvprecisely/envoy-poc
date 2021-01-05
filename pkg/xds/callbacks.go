package xds

import (
	"fmt"

	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_server_v3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/sirupsen/logrus"
)

// NewRequestLoggingCallbacks returns an implementation of the Envoy xDS server
// callbacks for use when Contour is run in Envoy xDS server mode to provide
// request detail logging. Currently only the xDS State of the World callback
// OnStreamRequest is implemented.
func NewRequestLoggingCallbacks(log logrus.FieldLogger) envoy_server_v3.Callbacks {
	return &envoy_server_v3.CallbackFuncs{
		StreamRequestFunc: func(streamID int64, req *envoy_service_discovery_v3.DiscoveryRequest) error {
			logDiscoveryRequestDetails(log, req)
			return nil
		},
	}
}

// Helper function for use in the Envoy xDS server callbacks and the Contour
// xDS server to log request details. Returns logger with fields added for any
// subsequent error handling and logging.
func logDiscoveryRequestDetails(l logrus.FieldLogger, req *envoy_service_discovery_v3.DiscoveryRequest) *logrus.Entry {
	log := l.WithField("version_info", req.VersionInfo).WithField("response_nonce", req.ResponseNonce)
	if req.Node != nil {
		log = log.WithField("node_id", req.Node.Id)

		if bv := req.Node.GetUserAgentBuildVersion(); bv != nil && bv.Version != nil {
			log = log.WithField("node_version", fmt.Sprintf("v%d.%d.%d", bv.Version.MajorNumber, bv.Version.MinorNumber, bv.Version.Patch))
		}
	}

	if status := req.ErrorDetail; status != nil {
		// if Envoy rejected the last update log the details here.
		// TODO(dfc) issue 1176: handle xDS ACK/NACK
		log.WithField("code", status.Code).Error(status.Message)
	}

	log = log.WithField("resource_names", req.ResourceNames).WithField("type_url", req.GetTypeUrl())
	log.Info("handling v3 xDS resource request")

	return log
}
