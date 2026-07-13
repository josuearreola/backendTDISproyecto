import os
import re
import hashlib
from PIL import Image
import pytesseract
from pdf2image import convert_from_path
import cv2
import numpy as np
import torch
from transformers import CLIPProcessor, CLIPModel

# Inicializar modelo de CLIP (se descargará localmente una sola vez al arrancar)
print("[Worker] Cargando modelo CLIP (openai/clip-vit-base-patch32) en CPU...")
clip_model = CLIPModel.from_pretrained("openai/clip-vit-base-patch32")
clip_processor = CLIPProcessor.from_pretrained("openai/clip-vit-base-patch32")
print("[Worker] Modelo CLIP cargado con éxito.")

# Ruta predeterminada de Tesseract para Windows por si no está en las variables de entorno
tesseract_paths = [
    r"C:\Program Files\Tesseract-OCR\tesseract.exe",
    r"C:\Program Files (x86)\Tesseract-OCR\tesseract.exe",
]
for path in tesseract_paths:
    if os.path.exists(path):
        pytesseract.pytesseract.tesseract_cmd = path
        break

def is_image_blurry(pil_image, threshold=80.0):
    """Calcula si la imagen está borrosa usando la Varianza del Laplaciano de OpenCV."""
    try:
        # Convertir imagen PIL a formato OpenCV (numpy array BGR)
        open_cv_image = np.array(pil_image)
        if len(open_cv_image.shape) == 3:
            open_cv_image = open_cv_image[:, :, ::-1].copy()
            gray = cv2.cvtColor(open_cv_image, cv2.COLOR_BGR2GRAY)
        else:
            gray = open_cv_image

        variance = cv2.Laplacian(gray, cv2.CV_64F).var()
        print(f"[Worker] Varianza del desenfoque detectada: {variance:.2f}")
        return variance < threshold
    except Exception as e:
        print(f"[Worker] Error al calcular desenfoque: {e}")
        return False

def check_image_with_clip(pil_image, expected_description):
    """Compara la imagen con la descripción esperada contra descripciones trampa usando CLIP."""
    try:
        # Definimos prompts para comparar (el esperado vs trampa)
        prompts = [
            f"a photo representing {expected_description}",
            "a random unrelated picture or document",
            "a blurry, blank, or completely dark image"
        ]
        
        inputs = clip_processor(text=prompts, images=pil_image, return_tensors="pt", padding=True)
        outputs = clip_model(**inputs)
        
        # Obtener probabilidades de similitud
        logits_per_image = outputs.logits_per_image
        probs = logits_per_image.softmax(dim=1).detach().numpy()[0]
        
        similarity_score = float(probs[0]) * 100.0
        print(f"[Worker] Similitud de CLIP para '{expected_description}': {similarity_score:.2f}%")
        return similarity_score
    except Exception as e:
        print(f"[Worker] Error en validación de CLIP: {e}")
        return 0.0

def process_file_pipeline(file_path, expected_description, student_name, matricula):
    """
    Pipeline principal de validación de 4 capas.
    Retorna (resultado_bool, coincidencia_score, observaciones_str)
    """
    ext = os.path.splitext(file_path)[1].lower()
    
    # 1. Extraer páginas como imágenes
    images = []
    try:
        if ext == ".pdf":
            images = convert_from_path(file_path, dpi=150)
        elif ext in [".png", ".jpg", ".jpeg"]:
            images = [Image.open(file_path)]
        else:
            return False, 0.0, "Formato no compatible para validación automática (Word/Excel). Requiere revisión manual."
    except Exception as e:
        return False, 0.0, f"Error al abrir archivo de evidencia: {str(e)}"

    if not images:
        return False, 0.0, "El archivo no contiene imágenes procesables."

    full_text = ""
    max_clip_score = 0.0
    blurry_pages = 0

    # Procesar página por página
    for i, img in enumerate(images):
        # Capa 2: Control de desenfoque (Calidad)
        if is_image_blurry(img):
            blurry_pages += 1
            continue

        # Capa 3: Verificación Semántica Visual (CLIP)
        clip_score = check_image_with_clip(img, expected_description)
        if clip_score > max_clip_score:
            max_clip_score = clip_score

        # Capa 4: Reconocimiento de texto (OCR)
        try:
            page_text = pytesseract.image_to_string(img, lang="spa")
            full_text += page_text + "\n"
        except Exception as e:
            print(f"[Worker] Advertencia: Error en OCR de página {i+1}: {e}")

    # Si hay páginas borrosas, se manda a revisión manual
    if blurry_pages > 0:
        return False, 0.0, f"La evidencia tiene {blurry_pages} página(s) ilegible(s) o borrosa(s)."

    # Capa 4: Validación de Identidad y Contenido
    def normalize(s):
        s = s.lower()
        replacements = [("á", "a"), ("é", "e"), ("í", "i"), ("ó", "o"), ("ú", "u"), ("ñ", "n")]
        for a, b in replacements:
            s = s.replace(a, b)
        return re.sub(r'[^a-z0-9\s]', '', s)

    norm_text = normalize(full_text)
    norm_student_name = normalize(student_name)
    norm_matricula = normalize(matricula)

    # Buscar nombre completo
    name_parts = [p for p in norm_student_name.split() if len(p) > 2]
    name_matched = False
    if name_parts:
        matches = sum(1 for part in name_parts if part in norm_text)
        if matches >= min(2, len(name_parts)):
            name_matched = True

    # Buscar matrícula
    matricula_matched = norm_matricula in norm_text if norm_matricula else False

    # Reglas para determinar dictamen final
    observaciones = []

    # Si hay texto legible (ej: certificados, listas), exigimos identidad
    if len(norm_text.strip()) > 30:
        if not name_matched and not matricula_matched:
            return False, 0.0, "Fallo de identidad: El documento de texto no contiene el nombre ni la matrícula del alumno."
        
        # Validar procedencia institucional (UTEQ)
        is_uteq = any(kw in norm_text for kw in ["uteq", "universidad tecnologica de queretaro"])
        if not is_uteq:
            observaciones.append("No se detectó el sello o membrete de la UTEQ en el texto")

    # Si es mayormente visual (ej: fotos de eventos), confiamos en el puntaje de CLIP
    # Un puntaje CLIP > 70% es una excelente coincidencia semántica
    if max_clip_score < 70.0:
        observaciones.append(f"La imagen no coincide suficientemente con la actividad requerida (CLIP: {max_clip_score:.1f}%)")

    # Si hay alguna observación o advertencia menor, pasa a revisión humana
    if observaciones:
        return False, max_clip_score, f"Revisión requerida: {', '.join(observaciones)}."

    return True, max_clip_score, "Validación automática aprobada con éxito."
