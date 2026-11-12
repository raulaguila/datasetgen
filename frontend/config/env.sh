#!/bin/bash
# Writes frontend/config/.env. Run from frontend/ (make init).

session_secret=$(openssl rand -hex 32)

echo "TZ='America/Manaus'                             # Set system time zone
WEB_VERSION='0.1.0'                             # Image tag
WEB_PORT='3006'                                 # Web PORT on the host
WEB_IP='172.30.97.10'                           # Container address; must equal WEB_IP of the backend stack

API_PORT='9977'                                 # Backend PORT; dev calls http://localhost:API_PORT, Docker http://backend:API_PORT
SESSION_SECRET='${session_secret}'              # Signs the session cookie - PRIVATE TOKEN
COOKIE_SECURE=''                                # Secure cookie: empty = only when served over HTTPS, 1 forces it, 0 disables it
CLIENT_IP_HEADER=''                             # Header with the client IP set by a reverse proxy (default x-forwarded-for)" >config/.env
