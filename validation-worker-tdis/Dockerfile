FROM python:3.11-slim

# Dependencias de sistema:
# - tesseract-ocr: usado por pytesseract (Capa 4: identidad/OCR)
# - poppler-utils: usado por pdf2image para convertir PDF -> imagenes
# - libglib2.0-0/libsm6/libxext6/libxrender1: requeridas en runtime por opencv-python-headless
RUN apt-get update && apt-get install -y --no-install-recommends tesseract-ocr poppler-utils libglib2.0-0 libsm6 libxext6 libxrender1 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Instalamos dependencias de Python primero para aprovechar la caché de capas.
# CLIP (torch + transformers) es opcional: no cabe en los 512 MB del plan gratuito.
# Para una imagen con CLIP: docker build --build-arg INSTALL_CLIP=true ...
ARG INSTALL_CLIP=false
COPY validation-worker-tdis/requirements.txt validation-worker-tdis/requirements-clip.txt ./
RUN pip install --no-cache-dir -r requirements.txt && \
    if [ "$INSTALL_CLIP" = "true" ]; then pip install --no-cache-dir -r requirements-clip.txt; fi

# Copiamos el código del worker.
COPY validation-worker-tdis/ .

# El worker consume validation_queue (BRPOP). Si existe PORT (Render) expone además /health,
# porque el plan gratuito solo admite web services.
CMD ["python", "main.py"]
