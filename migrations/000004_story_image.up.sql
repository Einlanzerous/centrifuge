-- CTFG-27: optional hero image per story. The reading UI uses the newsletter's
-- own image as the card/reader "art" header when one is available, falling back
-- to the generated topic gradient only when it is null. Population (extracting
-- the image from the newsletter HTML during scoring, or generating one) is a
-- follow-up; this column just makes the field first-class.
ALTER TABLE stories ADD COLUMN image_url text;
