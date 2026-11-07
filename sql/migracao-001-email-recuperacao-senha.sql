-- Migração para bancos criados antes do cadastro com e-mail e da recuperação
-- de senha. Rode uma vez no banco AgroStock (bancos novos já saem certos do
-- schema.sql).

ALTER TABLE Usuario ADD
    Email VARCHAR(255) NULL,
    ResetTokenHash CHAR(64) NULL,       -- SHA-256 do token de redefinição
    ResetTokenExpira DATETIME2 NULL;    -- em UTC
GO

CREATE UNIQUE INDEX UX_Usuario_Email ON Usuario (Email) WHERE Email IS NOT NULL;
CREATE INDEX IX_Usuario_ResetToken ON Usuario (ResetTokenHash) WHERE ResetTokenHash IS NOT NULL;
GO
