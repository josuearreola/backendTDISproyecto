package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// New crea un reverse proxy hacia la URL base de un servicio.
// stripPrefix es el prefijo de ruta que se elimina antes de reenviar:
// por ejemplo, una petición a /api/users/login con stripPrefix="/api/users"
// llega al servicio como /login.
func New(targetBase, stripPrefix string) (http.Handler, error) {
	target, err := url.Parse(targetBase)
	if err != nil {
		return nil, err
	}

	rp := httputil.NewSingleHostReverseProxy(target)

	// Personalizamos el Director para reescribir la ruta y el host.
	originalDirector := rp.Director
	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = singleJoiningSlash(target.Path, strings.TrimPrefix(req.URL.Path, stripPrefix))
		req.Host = target.Host
	}

	// Si el servicio downstream no responde, devolvemos 502 en JSON
	// en lugar del error HTML por defecto.
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"servicio no disponible"}`))
	}

	return rp, nil
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		if b == "" {
			return a
		}
		return a + "/" + b
	}
	return a + b
}
