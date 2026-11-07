CREATE TABLE Usuario (
    Id INT IDENTITY(1,1) PRIMARY KEY,
    Login VARCHAR(50) NOT NULL UNIQUE,
    HashSenha VARCHAR(255) NOT NULL,
    Ativo BIT NOT NULL DEFAULT 1,
    Email VARCHAR(255) NULL,
    ResetTokenHash CHAR(64) NULL,       -- SHA-256 do token de redefinição
    ResetTokenExpira DATETIME2 NULL     -- em UTC
);

CREATE UNIQUE INDEX UX_Usuario_Email ON Usuario (Email) WHERE Email IS NOT NULL;
CREATE INDEX IX_Usuario_ResetToken ON Usuario (ResetTokenHash) WHERE ResetTokenHash IS NOT NULL;

CREATE TABLE Insumo (
    Id INT IDENTITY(1,1) PRIMARY KEY,
    Nome VARCHAR(100) NOT NULL,
    Categoria VARCHAR(50) NOT NULL,
    Unidade VARCHAR(20) NOT NULL,
    Minimo DECIMAL(14,3) NOT NULL CHECK (Minimo >= 0)
);

CREATE TABLE Lote (
    Id INT IDENTITY(1,1) PRIMARY KEY,
    InsumoId INT NOT NULL,
    Codigo VARCHAR(50) NOT NULL,
    Validade DATE NOT NULL,
    Saldo DECIMAL(14,3) NOT NULL DEFAULT 0 CHECK (Saldo >= 0),
    CONSTRAINT FK_Lote_Insumo FOREIGN KEY (InsumoId) REFERENCES Insumo(Id),
    CONSTRAINT UQ_Lote_Insumo_Codigo UNIQUE (InsumoId, Codigo) 
);

CREATE TABLE Movimentacao (
    Id INT IDENTITY(1,1) PRIMARY KEY,
    LoteId INT NOT NULL,
    UsuarioId INT NOT NULL,
    Tipo CHAR(1) NOT NULL CHECK (Tipo IN ('E', 'S')),
    Quantidade DECIMAL(14,3) NOT NULL CHECK (Quantidade > 0),
    Instante DATETIME NOT NULL DEFAULT GETDATE(),
    Motivo VARCHAR(255) NULL,
    ChaveRequisicao VARCHAR(100) NOT NULL UNIQUE, 
    CONSTRAINT FK_Movimentacao_Lote FOREIGN KEY (LoteId) REFERENCES Lote(Id),
    CONSTRAINT FK_Movimentacao_Usuario FOREIGN KEY (UsuarioId) REFERENCES Usuario(Id)
);