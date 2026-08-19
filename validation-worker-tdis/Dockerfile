FROM python:3.11-slim

# Dependencias de sistema:
# - tesseract-ocr: usado por pytesseract (Capa 4: identidad/OCR)
# - poppler-utils: usado por pdf2image para convertir PDF -> imagenes
# - libglib2.0-0/libsm6/libxext6/libxrender1: requeridas en runtime por opencv-python-headless
RUN apt-get update && apt-get install -y --no-install-recommends tesseract-ocr poppler-utils libglib2.0-0 libsm6 libxext6 libxrender1 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Instalamos dependencias de Python primero para aprovechar la caché de capas.
COPY validation-worker-tdis/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copiamos el código del worker.
COPY validation-worker-tdis/ .

# El worker es un proceso de fondo (BRPOP sobre la cola de Redis), no expone HTTP.
CMD ["python", "main.py"]
