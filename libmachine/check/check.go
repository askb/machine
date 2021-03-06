package check

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/docker/machine/libmachine/auth"
	"github.com/docker/machine/libmachine/cert"
	"github.com/docker/machine/libmachine/host"
	"github.com/docker/machine/libmachine/state"
)

var (
	DefaultConnChecker ConnChecker
)

func init() {
	DefaultConnChecker = &MachineConnChecker{}
}

// ErrCertInvalid for when the cert is computed to be invalid.
type ErrCertInvalid struct {
	wrappedErr error
	hostURL    string
}

func (e ErrCertInvalid) Error() string {
	return fmt.Sprintf(`There was an error validating certificates for host %q: %s
You can attempt to regenerate them using 'docker-machine regenerate-certs [name]'.
Be advised that this will trigger a Docker daemon restart which will stop running containers.
`, e.hostURL, e.wrappedErr)
}

type ConnChecker interface {
	Check(*host.Host, bool) (dockerHost string, authOptions *auth.Options, err error)
}

type MachineConnChecker struct{}

func (mcc *MachineConnChecker) Check(h *host.Host, swarm bool) (string, *auth.Options, error) {
	hostState, err := h.Driver.GetState()
	if err != nil {
		// TODO: This is a common operation and should have a commonly
		// defined error.
		return "", &auth.Options{}, fmt.Errorf("Error trying to get host state: %s", err)
	}
	if hostState != state.Running {
		return "", &auth.Options{}, fmt.Errorf("%s is not running. Please start it in order to use the connection settings", h.Name)
	}

	dockerHost, err := h.Driver.GetURL()
	if err != nil {
		return "", &auth.Options{}, fmt.Errorf("Error getting driver URL: %s", err)
	}

	if swarm {
		var err error
		dockerHost, err = parseSwarm(dockerHost, h)
		if err != nil {
			return "", &auth.Options{}, fmt.Errorf("Error parsing swarm: %s", err)
		}
	}

	u, err := url.Parse(dockerHost)
	if err != nil {
		return "", &auth.Options{}, fmt.Errorf("Error parsing URL: %s", err)
	}

	authOptions := h.HostOptions.AuthOptions

	if err := checkCert(u.Host, authOptions); err != nil {
		return "", &auth.Options{}, fmt.Errorf("Error checking and/or regenerating the certs: %s", err)
	}

	return dockerHost, authOptions, nil
}

func checkCert(hostURL string, authOptions *auth.Options) error {
	valid, err := cert.ValidateCertificate(hostURL, authOptions)
	if !valid || err != nil {
		return ErrCertInvalid{
			wrappedErr: err,
			hostURL:    hostURL,
		}
	}

	return nil
}

// TODO: This could use a unit test.
func parseSwarm(hostURL string, h *host.Host) (string, error) {
	swarmOptions := h.HostOptions.SwarmOptions

	if !swarmOptions.Master {
		return "", fmt.Errorf("Error: %s is not a swarm master.  The --swarm flag is intended for use with swarm masters", h.Name)
	}

	u, err := url.Parse(swarmOptions.Host)
	if err != nil {
		return "", fmt.Errorf("There was an error parsing the url: %s", err)
	}
	parts := strings.Split(u.Host, ":")
	swarmPort := parts[1]

	// get IP of machine to replace in case swarm host is 0.0.0.0
	mURL, err := url.Parse(hostURL)
	if err != nil {
		return "", fmt.Errorf("There was an error parsing the url: %s", err)
	}

	mParts := strings.Split(mURL.Host, ":")
	machineIP := mParts[0]

	hostURL = fmt.Sprintf("tcp://%s:%s", machineIP, swarmPort)

	return hostURL, nil
}
