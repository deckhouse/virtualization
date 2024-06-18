import bcrypt

class Htpasswd:
    def __init__(self,
                 username: str,
                 password: str,) -> None:
        self.username = username
        self.password = password

    def generate(self) -> str:
        bcrypted = bcrypt.hashpw(self.password.encode("utf-8"), bcrypt.gensalt(prefix=b"2a")).decode("utf-8")
        return f"{self.username}:{bcrypted}"

    def validate(self, htpasswd: str) -> bool:
        user, hashed = htpasswd.strip().split(':')
        if user != self.username:
            return False
        return bcrypt.checkpw(self.password.encode("utf-8"), hashed.encode("utf-8"))