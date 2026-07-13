package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type Service struct {
	Name string
	Path string
	Port string
}

func main() {
	// 1. Cargar el archivo .env de la raíz
	envVars, err := loadRootEnv(".env")
	if err != nil {
		fmt.Printf(" Nota: No se pudo leer el archivo .env en la raíz (%v). Se usarán las variables del sistema.\n", err)
	} else {
		fmt.Println("Variables cargadas desde el archivo .env de la raíz.")
	}

		services := []Service{
		{Name: "GATEWAY     ", Path: "./api-gateaway-tdis", Port: "8080"},
		{Name: "USER-SERVICE", Path: "./user-service-tdis", Port: "8081"},
		{Name: "TDI-SERVICE ", Path: "./tdi-service-tdis", Port: "8082"},
	}


	fmt.Println("Arrancando todos los microservicios...")

	var wg sync.WaitGroup
	cmds := make([]*exec.Cmd, 0, len(services))
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var mu sync.Mutex

	for _, svc := range services {
		wg.Add(1)
		go func(s Service) {
			defer wg.Done()

			cmd := exec.Command("go", "run", ".")
			cmd.Dir = s.Path

			// Configurar el entorno del proceso hijo
			cmd.Env = os.Environ()
			// Añadir las variables leídas del .env raíz
			for k, v := range envVars {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
			// Inyectar el puerto correspondiente de manera específica
			cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%s", s.Port))

			stdout, err := cmd.StdoutPipe()
			if err != nil {
				fmt.Printf("[%s] Error al obtener stdout: %v\n", s.Name, err)
				return
			}
			stderr, err := cmd.StderrPipe()
			if err != nil {
				fmt.Printf("[%s] Error al obtener stderr: %v\n", s.Name, err)
				return
			}

			if err := cmd.Start(); err != nil {
				fmt.Printf("[%s] Error al iniciar: %v\n", s.Name, err)
				return
			}

			mu.Lock()
			cmds = append(cmds, cmd)
			mu.Unlock()

			go streamLogs(s.Name, stdout)
			go streamLogs(s.Name, stderr)

			fmt.Printf("[%s] Servicio iniciado con éxito en el puerto %s\n", s.Name, s.Port)

			if err := cmd.Wait(); err != nil {
				fmt.Printf("[%s] Detenido: %v\n", s.Name, err)
			} else {
				fmt.Printf("[%s] Finalizado correctamente\n", s.Name)
			}
		}(svc)
	}

	go func() {
		<-sigChan
		fmt.Println("\nRecibida señal de apagado. Deteniendo servicios...")
		mu.Lock()
		for _, cmd := range cmds {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		}
		mu.Unlock()
	}()

	wg.Wait()
	fmt.Println(" Todos los servicios se han detenido.")
}

func streamLogs(prefix string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Printf("[%s] %s\n", prefix, scanner.Text())
	}
}

// loadRootEnv lee un archivo .env simple y devuelve un mapa clave-valor.
func loadRootEnv(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Ignorar comentarios y líneas vacías
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Separar por el primer '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Limpiar comillas si las hay
			value = strings.Trim(value, `"'`)
			env[key] = value
		}
	}
	return env, scanner.Err()
}
