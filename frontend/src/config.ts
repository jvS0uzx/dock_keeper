// Base da API. Fora do localhost, definir VITE_API_URL no .env do frontend.
export const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';
