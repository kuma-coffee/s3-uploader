import { useState } from 'react';
import './App.css';

const CHUNK_SIZE = 5 * 1024 * 1024;

export default function App() {
  const [file, setFile] = useState(null);
  const [downloadUrl, setDownloadUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [status, setStatus] = useState('');

  const upload = async () => {
    if (!file) {
      alert('Pilih file terlebih dahulu');
      return;
    }

    try {
      setLoading(true);
      setProgress(0);
      setDownloadUrl('');

      console.group('🚀 RESUMABLE MULTIPART UPLOAD');

      const totalParts = Math.ceil(file.size / CHUNK_SIZE);

      console.log('FILE INFO:', {
        name: file.name,
        size: file.size,
        type: file.type,
        totalParts,
      });

      /*
        STEP 1

        START / RESUME UPLOAD
      */

      setStatus('Checking existing upload...');

      const initRes = await fetch('http://localhost:8080/upload/start', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          fileName: file.name,
          fileSize: file.size,
          totalParts,
        }),
      });

      console.log('START STATUS:', initRes.status);

      const initData = await initRes.json();

      console.log('START RESPONSE:', initData);

      /*
        PART YANG SUDAH ADA
      */

      const uploadedSet = new Set(initData.uploadedParts || []);

      console.log('EXISTING PARTS:', [...uploadedSet]);

      let completedParts = [];

      let uploadedCount = uploadedSet.size;

      setProgress(Math.round((uploadedCount / totalParts) * 100));

      /*
        STEP 2

        UPLOAD PART
      */

      setStatus('Uploading chunks...');

      const uploadPromises = initData.parts.map(async (part) => {
        /*
              PART SUDAH ADA
            */

        if (uploadedSet.has(part.partNumber)) {
          console.log(`⏭️ SKIP PART ${part.partNumber}`);

          return null;
        }

        console.log(`⬆️ UPLOAD PART ${part.partNumber}`);

        const start = (part.partNumber - 1) * CHUNK_SIZE;

        const end = Math.min(start + CHUNK_SIZE, file.size);

        const blob = file.slice(start, end);

        const uploadRes = await fetch(part.uploadUrl, {
          method: 'PUT',
          body: blob,
        });

        console.log(`PART ${part.partNumber} RESPONSE`, {
          status: uploadRes.status,

          headers: Object.fromEntries(uploadRes.headers.entries()),
        });

        if (!uploadRes.ok) {
          throw new Error(`Upload part ${part.partNumber} gagal`);
        }

        const etag = uploadRes.headers.get('ETag');

        if (!etag) {
          throw new Error(`ETag part ${part.partNumber} tidak ditemukan`);
        }

        uploadedCount++;

        setProgress(Math.round((uploadedCount / totalParts) * 100));

        return {
          partNumber: part.partNumber,

          etag,
        };
      });

      const uploadedNow = await Promise.all(uploadPromises);

      completedParts = uploadedNow.filter(Boolean);

      console.log('NEW COMPLETED PARTS:', completedParts);

      /*
        STEP 3

        COMPLETE UPLOAD
      */

      setStatus('Completing upload...');

      const completeRes = await fetch('http://localhost:8080/upload/complete', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          uploadId: initData.uploadId,

          objectKey: initData.objectKey,

          parts: completedParts,
        }),
      });

      console.log('COMPLETE STATUS:', completeRes.status);

      const completeData = await completeRes.json();

      console.log('COMPLETE RESPONSE:', completeData);

      setDownloadUrl(completeData.downloadUrl);

      setStatus('Upload selesai!');

      console.groupEnd();
    } catch (error) {
      console.error('UPLOAD ERROR:', error);

      setStatus('Upload gagal');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className='page'>
      <div className='card'>
        <h1>🔄 Resumable Upload</h1>

        <p className='subtitle'>Upload file besar dengan kemampuan resume</p>

        <div className='upload-box'>
          <input
            type='file'
            onChange={(e) => {
              if (e.target.files) {
                setFile(e.target.files[0]);
              }
            }}
          />

          {file && (
            <div className='file-info'>
              <b>File:</b>

              <br />

              {file.name}

              <br />

              <small>
                {(file.size / 1024 / 1024).toFixed(2)}
                MB
              </small>
            </div>
          )}
        </div>

        <div className='progress-container'>
          <div
            className='progress-bar'
            style={{
              width: `${progress}%`,
            }}
          />
        </div>

        <div className='progress-text'>{progress}%</div>

        <p className='status'>{status}</p>

        <button onClick={upload} disabled={loading}>
          {loading ? 'Uploading...' : 'Start Upload'}
        </button>

        {downloadUrl && (
          <div className='preview'>
            <h3>✅ Upload berhasil</h3>

            <label>Preview URL:</label>

            <a href={downloadUrl} target='_blank' rel='noreferrer'>
              {downloadUrl}
            </a>
          </div>
        )}
      </div>
    </div>
  );
}
