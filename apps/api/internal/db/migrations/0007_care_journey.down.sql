-- Reverte 0007_care_journey. As duas tabelas são independentes entre si (não há FK
-- de uma para a outra); os índices caem junto com elas.
DROP TABLE IF EXISTS journey_event;
DROP TABLE IF EXISTS care_appointment;
