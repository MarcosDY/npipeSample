FROM mcr.microsoft.com/windows/servercore:ltsc2022 AS npipe-server
COPY server/server.exe c:/
ENTRYPOINT ["c:/server.exe"]
CMD []

FROM mcr.microsoft.com/windows/servercore:ltsc2022 AS npipe-client
COPY client/client.exe c:/
ENTRYPOINT ["c:/client.exe", "run"]
CMD []
